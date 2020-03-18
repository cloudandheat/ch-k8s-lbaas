package agent

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/klog"

	jwt "github.com/dgrijalva/jwt-go"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
)

type ApplyHandlerv1 struct {
	mutex sync.Mutex

	KeepalivedConfig *ConfigManager
	NftablesConfig   *ConfigManager
	MaxRequestSize   int64
	SharedSecret     []byte
}

type ConfigManager struct {
	Generator ConfigGenerator
	Service   config.ServiceConfig
}

func diffFiles(oldFile, newFile string) (changed bool, diff string, err error) {
	output := &strings.Builder{}
	cmd := exec.Command("diff", "-U3", oldFile, newFile)
	cmd.Stdout = output
	err = cmd.Run()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			return false, "", err
		} else {
			changed = true
		}
	}
	diff = output.String()
	return changed, output.String(), nil
}

func (m *ConfigManager) MakeBackup() (string, error) {
	fin, err := os.Open(m.Service.ConfigFile)
	if err != nil {
		if err != os.ErrNotExist {
			return "", err
		} else {
			// nothing to take a backup of
			return "", nil
		}
	}
	defer fin.Close()

	dir := filepath.Dir(m.Service.ConfigFile)
	fout, err := ioutil.TempFile(dir, ".bak-*")
	if err != nil {
		return "", err
	}
	defer fout.Close()
	_, err = io.Copy(fout, fin)

	if err != nil {
		os.Remove(fout.Name())
		return "", err
	}

	return fout.Name(), nil
}

func (m *ConfigManager) Reload() error {
	klog.V(4).Infof("executing reload: %#v", m.Service.ReloadCommand)
	cmd := m.Service.ReloadCommand
	return exec.Command(cmd[0], cmd[1:]...).Run()
}

func (m *ConfigManager) Check() error {
	klog.V(4).Infof("executing check: %#v", m.Service.StatusCommand)
	cmd := m.Service.StatusCommand
	if cmd == nil || len(cmd) == 0 {
		// no check supported, assume the best
		return nil
	}
	return exec.Command(cmd[0], cmd[1:]...).Run()
}

func (m *ConfigManager) Fix() error {
	klog.V(3).Infof("trying to fix broken service via reload and %#v", m.Service.StartCommand)
	err := m.Reload()
	cmd := m.Service.StartCommand
	if cmd == nil || len(cmd) == 0 {
		return err
	}
	// deliberately ignoring the Reload() error here; the idea behind this
	// is that a reload of a crashed service will fail, however, the
	// FixCommand may restart the service.
	return exec.Command(cmd[0], cmd[1:]...).Run()
}

func (m *ConfigManager) ReloadAndCheck() error {
	klog.V(3).Infof("checked reload of service via %#v", m.Service.ReloadCommand)
	err := m.Reload()
	if err != nil {
		return err
	}

	if m.Service.CheckDelay > 0 {
		time.Sleep(time.Duration(m.Service.CheckDelay) * time.Second)
	}

	return m.Check()
}

func (m *ConfigManager) WriteWithRollback(cfg *model.LoadBalancer) (bool, error) {
	klog.V(1).Infof("writing configuration file %s", m.Service.ConfigFile)

	dir := filepath.Dir(m.Service.ConfigFile)
	fout, err := ioutil.TempFile(dir, ".tmp-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(fout.Name())

	err = func() error {
		defer fout.Close()
		return m.Generator.GenerateConfig(cfg, fout)
	}()
	if err != nil {
		return false, err
	}

	backupFile, err := m.MakeBackup()
	if err != nil {
		return false, err
	}
	defer os.Remove(backupFile)

	var changed bool
	if backupFile != "" {
		var diff string
		changed, diff, err = diffFiles(backupFile, fout.Name())
		if changed {
			klog.Infof("configuration diff for %s:\n%s", m.Service.ConfigFile, diff)
		}
	} else {
		changed = true
		klog.V(2).Infof("no old configuration file\n")
	}

	if !changed {
		klog.V(1).Infof("configuration had no changes, skipping reload")
		return false, nil
	}

	// all files in place, do the swappety swap
	os.Rename(fout.Name(), m.Service.ConfigFile)
	klog.V(1).Infof("updating configuration file %s", m.Service.ConfigFile)
	err = m.ReloadAndCheck()
	if err != nil {
		// failed -> swap back
		if backupFile != "" {
			restoreErr := os.Rename(backupFile, m.Service.ConfigFile)
			if restoreErr != nil {
				klog.Warningf("failed to restore config backup: %s!", restoreErr.Error())
			}
		} else {
			// no backup exists, we delete the new file
			restoreErr := os.Remove(m.Service.ConfigFile)
			if restoreErr != nil {
				klog.Warningf("failed to remove invalid config: %s!", restoreErr.Error())
			}
		}

		fixErr := m.Fix()
		if fixErr != nil {
			klog.Errorf("failed to recover broken service: %s!", fixErr.Error())
		}

		return false, err
	}

	return true, nil
}

func (h *ApplyHandlerv1) preflightCheck(w http.ResponseWriter, r *http.Request) (content_length int64, success bool) {
	if r.Method != "POST" {
		w.WriteHeader(405) // Method Not Allowed
		return content_length, false
	}

	content_type := r.Header.Get("Content-Type")
	mediatype, _, err := mime.ParseMediaType(content_type)
	if err != nil {
		w.WriteHeader(400) // Bad Request
		return content_length, false
	}

	if mediatype != "application/jwt" {
		w.WriteHeader(415) // Unsupported Media Type
		return content_length, false
	}

	content_length_s := r.Header.Get("Content-Length")
	content_length, err = strconv.ParseInt(content_length_s, 10, 32)
	if err != nil {
		w.WriteHeader(400) // Bad Request
		return content_length, false
	}

	if content_length > h.MaxRequestSize {
		w.WriteHeader(413) // Request Entity Too Large
		return content_length, false
	}

	return content_length, true
}

func (h *ApplyHandlerv1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	klog.V(5).Infof("incoming request from %s", r.RemoteAddr)

	size, ok := h.preflightCheck(w, r)
	if !ok {
		klog.V(5).Infof("request from %s did not pass preflight", r.RemoteAddr)
		return
	}

	body_buffer := &strings.Builder{}
	received, err := io.CopyN(body_buffer, r.Body, size)
	if (err != nil && err != io.EOF) || received < size {
		klog.V(5).Infof("Failed to read full request body. Bytes read %d (expected %d), error %s", received, size, err.Error())
		w.WriteHeader(400) // Bad Request
		return
	}

	claims := &model.ConfigClaim{}
	token, err := jwt.ParseWithClaims(body_buffer.String(), claims, func(*jwt.Token) (interface{}, error) {
		return h.SharedSecret, nil
	})
	if err != nil {
		klog.V(5).Infof("Failed to parse token: %s", err.Error())
		w.WriteHeader(400) // Bad Request
		return
	}
	if !token.Valid {
		klog.V(5).Infof("Failed to validate token")
		w.WriteHeader(401) // Unauthorized
		return
	}

	claims, ok = token.Claims.(*model.ConfigClaim)
	if !ok {
		klog.Warning("Failed to decode request from token")
		w.WriteHeader(400) // Unauthorized
		return
	}

	w.Header().Add("Content-Type", "text/plain")

	lbcfg := &claims.Config

	klog.V(1).Infof("received config: %#v", lbcfg)

	changed, err := h.KeepalivedConfig.WriteWithRollback(lbcfg)
	if err != nil {
		msg := fmt.Sprintf("Failed to apply keepalived config: %s", err.Error())
		klog.Error(msg)
		w.WriteHeader(500)
		w.Write([]byte(msg))
		return
	}

	changed, err = h.NftablesConfig.WriteWithRollback(lbcfg)
	if err != nil {
		msg := fmt.Sprintf("Failed to apply nftables config: %s", err.Error())
		klog.Error(msg)
		w.WriteHeader(500)
		w.Write([]byte(msg))
		return
	}

	if changed {
		klog.Infof("Applied configuration update: %#v", lbcfg)
	}

	w.WriteHeader(200)
	w.Write([]byte("success"))
}
