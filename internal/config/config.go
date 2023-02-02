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
package config

import (
	"fmt"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/static"
	"io"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

type BackendLayer string

const (
	BackendLayerNodePort  BackendLayer = "NodePort"
	BackendLayerClusterIP BackendLayer = "ClusterIP"
	BackendLayerPod       BackendLayer = "Pod"
)

type PortManager string

const (
	PortManagerOpenstack PortManager = "openstack"
	PortManagerStatic    PortManager = "static"
)

type Agent struct {
	URL string `toml:"url"`
}

type ServiceConfig struct {
	ConfigFile    string   `toml:"config-file"`
	ReloadCommand []string `toml:"reload-command"`
	StatusCommand []string `toml:"status-command"`
	StartCommand  []string `toml:"start-command"`
	CheckDelay    int      `toml:"check-delay"`
}

type Keepalived struct {
	Enabled bool `toml:"enabled"`

	VRRPPassword string `toml:"vrrp-password"`
	// TODO: allow different priorities per-service so that inbound traffic
	// is being balanced between VMs.
	Priority  int    `toml:"priority"`
	VRIDBase  int    `toml:"virtual-router-id-base"`
	Interface string `toml:"interface"`

	Service ServiceConfig `toml:"service"`
}

type Nftables struct {
	FilterTableName         string   `toml:"filter-table-name"`
	FilterTableType         string   `toml:"filter-table-type"`
	FilterForwardChainName  string   `toml:"filter-forward-chain"`
	NATTableName            string   `toml:"nat-table-name"`
	NATPreroutingChainName  string   `toml:"nat-prerouting-chain"`
	NATPostroutingChainName string   `toml:"nat-postrouting-chain"`
	PolicyPrefix            string   `toml:"policy-prefix"`
	NftCommand              []string `toml:"nft-command"`
	PartialReload           bool     `toml:"partial-reload"`
	EnableSNAT              bool     `toml:"enable-snat"`
	FWMarkBits              uint32   `toml:"fwmark-bits"`
	FWMarkMask              uint32   `toml:"fwmark-mask"`

	Service ServiceConfig `toml:"service"`
}

type Agents struct {
	SharedSecret  string  `toml:"shared-secret"`
	TokenLifetime int     `toml:"token-lifetime"`
	Agents        []Agent `toml:"agent"`
}

type ControllerConfig struct {
	BindAddress string `toml:"bind-address"`
	BindPort    int32  `toml:"bind-port"`

	PortManager  PortManager  `toml:"port-manager"`
	BackendLayer BackendLayer `toml:"backend-layer"`

	OpenStack openstack.Config `toml:"openstack"`
	Static    static.Config    `toml:"static"`
	Agents    Agents           `toml:"agents"`
}

type AgentConfig struct {
	SharedSecret string `toml:"shared-secret"`
	BindAddress  string `toml:"bind-address"`
	BindPort     int32  `toml:"bind-port"`

	Keepalived Keepalived `toml:"keepalived"`
	Nftables   Nftables   `toml:"nftables"`
}

func ReadControllerConfig(configReader io.Reader, config *ControllerConfig) error {
	_, err := toml.DecodeReader(configReader, &config)
	return err
}

func ReadControllerConfigFromFile(path string, withDefaults bool) (ControllerConfig, error) {
	fin, err := os.Open(path)
	if err != nil {
		return ControllerConfig{}, err
	}
	defer fin.Close()

	config := ControllerConfig{}
	if withDefaults {
		// Fill config before decoding toml to allow boolean default values
		// See https://github.com/BurntSushi/toml/issues/171
		FillControllerConfig(&config)
	}

	err = ReadControllerConfig(fin, &config)
	if err != nil {
		return ControllerConfig{}, err
	}

	return config, nil
}

func ReadAgentConfig(configFile io.Reader, config *AgentConfig) error {
	_, err := toml.DecodeReader(configFile, &config)
	return err
}

func ReadAgentConfigFromFile(path string, withDefaults bool) (AgentConfig, error) {
	fin, err := os.Open(path)
	if err != nil {
		return AgentConfig{}, err
	}
	defer fin.Close()

	config := AgentConfig{}
	if withDefaults {
		// Fill config before decoding toml to allow boolean default values
		// See https://github.com/BurntSushi/toml/issues/171
		FillAgentConfig(&config)
	}

	err = ReadAgentConfig(fin, &config)
	if err != nil {
		return AgentConfig{}, err
	}

	return config, nil
}

func FillKeepalivedConfig(cfg *Keepalived) {
	cfg.Enabled = true
	cfg.VRRPPassword = "useless"

	cfg.Service.ReloadCommand = []string{"sudo", "systemctl", "reload", "keepalived"}
	cfg.Service.StatusCommand = []string{"sudo", "systemctl", "is-active", "keepalived"}
	cfg.Service.StartCommand = []string{"sudo", "systemctl", "start", "keepalived"}
}

func FillNftablesConfig(cfg *Nftables) {
	cfg.FilterTableName = "filter"
	cfg.FilterTableType = "inet"
	cfg.FilterForwardChainName = "forward"
	cfg.NATTableName = "nat"
	cfg.NATPreroutingChainName = "prerouting"
	cfg.NATPostroutingChainName = "postrouting"
	cfg.NftCommand = []string{"sudo", "nft"}
	cfg.EnableSNAT = true

	if cfg.FWMarkBits == 0 {
		cfg.FWMarkBits = 1
		cfg.FWMarkMask = 1
	}

	cfg.Service.ReloadCommand = []string{"sudo", "systemctl", "reload", "nftables"}
	cfg.Service.StartCommand = []string{"sudo", "systemctl", "restart", "nftables"}
}

func FillAgentConfig(cfg *AgentConfig) {
	FillKeepalivedConfig(&cfg.Keepalived)
	FillNftablesConfig(&cfg.Nftables)
}

func FillControllerConfig(cfg *ControllerConfig) {
	cfg.PortManager = PortManagerOpenstack
	cfg.BindPort = 15203
	cfg.BackendLayer = BackendLayerNodePort
}

func ValidateControllerConfig(cfg *ControllerConfig) error {
	switch cfg.BackendLayer {
	case BackendLayerClusterIP:
		break
	case BackendLayerNodePort:
		break
	case BackendLayerPod:
		break
	default:
		return fmt.Errorf("backend-layer has an invalid value: %q", cfg.BackendLayer)
	}

	if cfg.PortManager == PortManagerOpenstack {
		// TODO: Add openstack config validation.
	} else if cfg.PortManager == PortManagerStatic {
		if len(cfg.Static.IPv4Addresses) == 0 {
			return fmt.Errorf("static.ipv4-addresses must have at least one " +
				"entry if static port manager is used")
		} else {
			for _, addr := range cfg.Static.IPv4Addresses {
				if !addr.Is4() {
					return fmt.Errorf("%s isn't a valid IPv4 address", addr.String())
				}
			}
		}
	} else {
		return fmt.Errorf("%s is not a valid port-manager implementation", cfg.PortManager)
	}

	return nil
}

func ValidateAgentConfig(cfg *AgentConfig) error {
	if cfg.Keepalived.Enabled {
		if cfg.Keepalived.VRIDBase <= 0 {
			return fmt.Errorf("keepalived.virtual-router-id-base must be greater than zero")
		}

		if cfg.Keepalived.Priority < 0 {
			return fmt.Errorf("keepalived.priority must be non-negative")
		}

		if cfg.Keepalived.Interface == "" {
			return fmt.Errorf("keepalived.interface must be set")
		}

		if cfg.Keepalived.Service.ConfigFile == "" {
			return fmt.Errorf("keepalived.service.config-file must be set")
		}
	}

	if cfg.Nftables.Service.ConfigFile == "" {
		return fmt.Errorf("nftables.service.config-file must be set")
	}

	if cfg.Nftables.PartialReload {
		if cfg.Nftables.PolicyPrefix == "" {
			return fmt.Errorf("nftables.policy-prefix must be set if partial-reload is enabled")
		}
	}

	if cfg.SharedSecret == "" {
		return fmt.Errorf("shared-secret must be set")
	}

	if cfg.BindAddress == "" {
		return fmt.Errorf("bind-address must be set")
	}

	if cfg.BindPort == 0 {
		return fmt.Errorf("bind-port must be set")
	}

	return nil
}
