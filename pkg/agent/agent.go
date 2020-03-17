package agent

import (
	"bytes"
	"os"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"sync"

	"k8s.io/klog"

	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
)

type ApplyHandlerv1 struct {
	mutex sync.Mutex

	KeepalivedGenerator ConfigGenerator
	KeepalivedOutputFile string
	MaxRequestSize int
}

func (h *ApplyHandlerv1) preflightCheck(w http.ResponseWriter, r *http.Request) (content_length int, success bool) {
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
	content_length_i64, err := strconv.ParseInt(content_length_s, 10, 32)
	if err != nil {
		w.WriteHeader(400) // Bad Request
		return content_length, false
	}
	content_length = int(content_length_i64)

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

	body_buffer := make([]byte, size)
	nread, err := r.Body.Read(body_buffer)
	if (err != nil && err != io.EOF) || nread < size {
		klog.V(5).Infof("Failed to read full request body. Bytes read %d (expected %d), error %s", nread, size, err.Error())
		w.WriteHeader(400) // Bad Request
		return
	}

	// TODO: signature validation and stuff
	reader := bytes.NewReader(body_buffer)
	lbcfg := &model.LoadBalancer{}
	err = json.NewDecoder(reader).Decode(lbcfg)
	if err != nil {
		klog.Warningf("Failed to decode request: %s", err.Error())
		w.WriteHeader(400) // Bad Request
		return
	}

	klog.Infof("received config: %#v", lbcfg)

	fout, err := os.OpenFile(h.KeepalivedOutputFile, os.O_WRONLY | os.O_TRUNC | os.O_CREATE, 0750)
	if err != nil {
		klog.Errorf("Failed to open keepalived config for writing: %s", err.Error())
		w.WriteHeader(500)
		return
	}
	defer fout.Close()

	err = h.KeepalivedGenerator.GenerateConfig(lbcfg, fout)
	if err != nil {
		klog.Errorf("Failed to generate keepalived config: %s", err.Error())
		w.WriteHeader(500)
		return
	}

	w.WriteHeader(200)
}
