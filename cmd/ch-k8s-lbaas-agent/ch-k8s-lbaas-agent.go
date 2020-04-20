package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"net/http"
	"time"

	"k8s.io/klog"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/agent"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
)

var (
	configPath string
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	fileCfg, err := config.ReadAgentConfigFromFile(configPath)
	if err != nil {
		klog.Fatalf("Failed reading config: %s", err.Error())
	}

	config.FillAgentConfig(&fileCfg)
	err = config.ValidateAgentConfig(&fileCfg)
	if err != nil {
		klog.Fatalf("invalid configuration: %s", err.Error())
	}

	sharedSecret, err := base64.StdEncoding.DecodeString(fileCfg.SharedSecret)
	if err != nil {
		klog.Fatalf("shared-secret failed to decode: %s", err.Error())
	}

	nftablesConfig := &agent.ConfigManager{
		Service: fileCfg.Nftables.Service,
		Generator: &agent.NftablesGenerator{
			Cfg: fileCfg.Nftables,
		},
	}

	keepalivedConfig := &agent.ConfigManager{
		Service: fileCfg.Keepalived.Service,
		Generator: &agent.KeepalivedConfigGenerator{
			VRIDBase:     fileCfg.Keepalived.VRIDBase,
			VRRPPassword: fileCfg.Keepalived.VRRPPassword,
			Interface:    fileCfg.Keepalived.Interface,
			Priority:     fileCfg.Keepalived.Priority,
		},
	}

	http.Handle("/v1/apply", &agent.ApplyHandlerv1{
		MaxRequestSize:   1048576,
		SharedSecret:     sharedSecret,
		KeepalivedConfig: keepalivedConfig,
		NftablesConfig:   nftablesConfig,
	})

	http.Handle("/metrics", promhttp.Handler())

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", fileCfg.BindAddress, fileCfg.BindPort))
	if err != nil {
		klog.Fatalf("Failed to set up HTTP listener: %s", err.Error())
	}

	s := &http.Server{
		Handler: nil,
		ReadTimeout: 2 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		IdleTimeout: 10 * time.Second,
	}

	s.Serve(listener)
}

func init() {
	flag.StringVar(&configPath, "config", "agent-config.toml", "Path to the agent config file.")
}
