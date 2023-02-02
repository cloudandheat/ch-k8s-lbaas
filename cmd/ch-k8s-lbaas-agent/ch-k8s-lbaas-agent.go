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

	fileCfg, err := config.ReadAgentConfigFromFile(configPath, true)
	if err != nil {
		klog.Fatalf("Failed reading config: %s", err.Error())
	}

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

	var keepalivedConfig *agent.ConfigManager

	if fileCfg.Keepalived.Enabled {
		keepalivedConfig = &agent.ConfigManager{
			Service: fileCfg.Keepalived.Service,
			Generator: &agent.KeepalivedConfigGenerator{
				VRIDBase:     fileCfg.Keepalived.VRIDBase,
				VRRPPassword: fileCfg.Keepalived.VRRPPassword,
				Interface:    fileCfg.Keepalived.Interface,
				Priority:     fileCfg.Keepalived.Priority,
			},
		}
	}

	// If PartialReload is enabled, reload nftables config directly after start to apply last state
	if fileCfg.Nftables.PartialReload {
		nftablesConfig.Reload()
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
		Handler:           nil,
		ReadTimeout:       2 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		IdleTimeout:       10 * time.Second,
	}

	s.Serve(listener)
}

func init() {
	flag.StringVar(&configPath, "config", "agent-config.toml", "Path to the agent config file.")
}
