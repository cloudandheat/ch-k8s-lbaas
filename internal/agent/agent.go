/* Copyright 2020 CLOUD&HEAT Technologies GmbH
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
)

var (
	metricLastUpdateTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ch_k8s_lbaas_agent_last_update_timestamp_seconds",
			Help: "Timestamp of the last update by status code",
		},
		[]string{"status"},
	)
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
	cmd.Stderr = os.Stderr
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
		if os.IsNotExist(err) {
			// nothing to take a backup of
			return "", nil
		} else {
			return "", err
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
	cmdObj := exec.Command(cmd[0], cmd[1:]...)
	cmdObj.Stderr = os.Stderr
	err := cmdObj.Run()
	if err != nil {
		return fmt.Errorf("failed to reload service via %#v: %s", m.Service.ReloadCommand, err.Error())
	}
	return nil
}

func (m *ConfigManager) Check() error {
	klog.V(4).Infof("executing check: %#v", m.Service.StatusCommand)
	cmd := m.Service.StatusCommand
	if cmd == nil || len(cmd) == 0 {
		// no check supported, assume the best
		return nil
	}
	cmdObj := exec.Command(cmd[0], cmd[1:]...)
	cmdObj.Stderr = os.Stderr
	return cmdObj.Run()
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
	cmdObj := exec.Command(cmd[0], cmd[1:]...)
	cmdObj.Stderr = os.Stderr
	err = cmdObj.Run()
	if err != nil {
		return fmt.Errorf("failed to fix service via %#v: %s", m.Service.ReloadCommand, err.Error())
	}
	return nil
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
		return false, fmt.Errorf("failed to create backup of current configuration: %s", err.Error())
	}
	defer os.Remove(backupFile)

	var changed bool
	if backupFile != "" {
		var diff string
		changed, diff, err = diffFiles(backupFile, fout.Name())
		if err != nil {
			return false, fmt.Errorf("failed diff config: %s", err.Error())
		}
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

func (h *ApplyHandlerv1) ProcessRequest(lbcfg *model.LoadBalancer) (int, string) {
	klog.V(1).Infof("received config: %#v", lbcfg)

	changed, err := h.KeepalivedConfig.WriteWithRollback(lbcfg)
	if err != nil {
		msg := fmt.Sprintf("Failed to apply keepalived config: %s", err.Error())
		klog.Error(msg)
		return 500, msg
	}

	changed, err = h.NftablesConfig.WriteWithRollback(lbcfg)
	if err != nil {
		msg := fmt.Sprintf("Failed to apply nftables config: %s", err.Error())
		klog.Error(msg)
		return 500, msg
	}

	if changed {
		klog.Infof("Applied configuration update: %#v", lbcfg)
	}

	return 200, "success"
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

	// Only at this point do we take the request seriously. Before this point,
	// it could’ve been an unauthenticated attacker.

	claims, ok = token.Claims.(*model.ConfigClaim)
	if !ok {
		klog.Warning("Failed to decode request from token")
		w.WriteHeader(400) // Bad Request
		return
	}

	w.Header().Add("Content-Type", "text/plain")

	status, body := h.ProcessRequest(&claims.Config)
	w.WriteHeader(status)
	w.Write([]byte(body))
	metricLastUpdateTimestamp.With(prometheus.Labels{"status": strconv.FormatInt(int64(status), 10)}).Set(float64(time.Now().UnixNano()) / 1000000000)
}
