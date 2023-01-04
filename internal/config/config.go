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
	LiveReload              bool     `toml:"live-reload"`
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
	PortManager string `toml:"port-manager"` // "openstack" or "static"
	BindPort    int32  `toml:"bind-port"`

	OpenStack    openstack.Config `toml:"openstack"`
	Static       static.Config    `toml:"static"`
	Agents       Agents           `toml:"agents"`
	BackendLayer BackendLayer     `toml:"backend-layer"`
}

type AgentConfig struct {
	SharedSecret string `toml:"shared-secret"`
	BindAddress  string `toml:"bind-address"`
	BindPort     int32  `toml:"bind-port"`

	Keepalived Keepalived `toml:"keepalived"`
	Nftables   Nftables   `toml:"nftables"`
}

func ReadControllerConfig(config io.Reader) (result ControllerConfig, err error) {
	_, err = toml.DecodeReader(config, &result)
	return result, err
}

func ReadControllerConfigFromFile(path string) (ControllerConfig, error) {
	fin, err := os.Open(path)
	if err != nil {
		return ControllerConfig{}, err
	}
	defer fin.Close()
	return ReadControllerConfig(fin)
}

func ReadAgentConfig(config io.Reader) (result AgentConfig, err error) {
	_, err = toml.DecodeReader(config, &result)
	return result, err
}

func ReadAgentConfigFromFile(path string) (AgentConfig, error) {
	fin, err := os.Open(path)
	if err != nil {
		return AgentConfig{}, err
	}
	defer fin.Close()
	return ReadAgentConfig(fin)
}

func defaultString(field *string, value string) {
	if *field == "" {
		*field = value
	}
}

func defaultStringList(field *[]string, value []string) {
	if field == nil || len(*field) == 0 {
		*field = make([]string, len(value))
		copy(*field, value)
	}
}

func defaultTrue(field *bool) {
	*field = true
}

func FillKeepalivedConfig(cfg *Keepalived) {
	defaultTrue(&cfg.Enabled)
	defaultString(&cfg.VRRPPassword, "useless")

	defaultStringList(&cfg.Service.ReloadCommand, []string{"sudo", "systemctl", "reload", "keepalived"})
	defaultStringList(&cfg.Service.StatusCommand, []string{"sudo", "systemctl", "is-active", "keepalived"})
	defaultStringList(&cfg.Service.StartCommand, []string{"sudo", "systemctl", "start", "keepalived"})
}

func FillNftablesConfig(cfg *Nftables) {
	defaultString(&cfg.FilterTableName, "filter")
	defaultString(&cfg.FilterTableType, "inet")
	defaultString(&cfg.FilterForwardChainName, "forward")
	defaultString(&cfg.NATTableName, "nat")
	defaultString(&cfg.NATPreroutingChainName, "prerouting")
	defaultString(&cfg.NATPostroutingChainName, "postrouting")
	defaultStringList(&cfg.NftCommand, []string{"sudo", "nft"})

	if cfg.FWMarkBits == 0 {
		cfg.FWMarkBits = 1
		cfg.FWMarkMask = 1
	}

	defaultStringList(&cfg.Service.ReloadCommand, []string{"sudo", "systemctl", "reload", "nftables"})
	defaultStringList(&cfg.Service.StartCommand, []string{"sudo", "systemctl", "restart", "nftables"})
}

func FillAgentConfig(cfg *AgentConfig) {
	FillKeepalivedConfig(&cfg.Keepalived)
	FillNftablesConfig(&cfg.Nftables)
}

func FillControllerConfig(cfg *ControllerConfig) {
	defaultString(&cfg.PortManager, "openstack")

	if cfg.BindPort == 0 {
		cfg.BindPort = 15203
	}

	if cfg.BackendLayer == "" {
		cfg.BackendLayer = BackendLayerNodePort
	}
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

	if cfg.PortManager == "openstack" {
		// TODO: Add openstack config validation.
	} else if cfg.PortManager == "static" {
		if len(cfg.Static.IPv4Addresses) == 0 {
			return fmt.Errorf("static.ipv4-addresses must have at least one " +
				"entry if static port manager is used")
		}
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
