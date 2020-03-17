package config

import (
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/cloudandheat/cah-loadbalancer/pkg/openstack"
)

type Agent struct {
	Address string `toml:"address"`
	Port    int32  `toml:"port"`
}

type Keepalived struct {
	VRRPPassword string `toml:"vrrp-password"`
	// TODO: allow different priorities per-service so that inbound traffic
	// is being balanced between VMs.
	Priority   int    `toml:"priority"`
	VRIDBase   int    `toml:"virtual-router-id-base"`
	Interface  string `toml:"interface"`
	OutputFile string `toml:"output-file"`
}

type Nftables struct {
	FilterTableName         string `toml:"filter-table-name"`
	FilterTableType         string `toml:"filter-table-type"`
	FilterForwardChainName  string `toml:"filter-forward-chain"`
	NATTableName            string `toml:"nat-table-name"`
	NATPreroutingChainName  string `toml:"nat-prerouting-chain"`
	NATPostroutingChainName string `toml:"nat-postrouting-chain"`
	FWMarkBits              uint32 `toml:"fwmark-bits"`
	FWMarkMask              uint32 `toml:"fwmark-mask"`
	OutputFile              string `toml:"output-file"`
}

type Agents struct {
	SharedSecret string  `toml:"shared-secret"`
	Agents       []Agent `toml:"agent"`
}

type ControllerConfig struct {
	OpenStack openstack.Config `toml:"openstack"`
	Agents    Agents           `toml:"agents"`
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

func FillKeepalivedConfig(cfg *Keepalived) {
	defaultString(&cfg.VRRPPassword, "useless")
}

func FillNftablesConfig(cfg *Nftables) {
	defaultString(&cfg.FilterTableName, "filter")
	defaultString(&cfg.FilterTableType, "inet")
	defaultString(&cfg.FilterForwardChainName, "forward")
	defaultString(&cfg.NATTableName, "nat")
	defaultString(&cfg.NATPreroutingChainName, "prerouting")
	defaultString(&cfg.NATPostroutingChainName, "postrouting")

	if cfg.FWMarkBits == 0 {
		cfg.FWMarkBits = 1
		cfg.FWMarkMask = 1
	}
}

func FillAgentConfig(cfg *AgentConfig) {
	FillKeepalivedConfig(&cfg.Keepalived)
	FillNftablesConfig(&cfg.Nftables)
}

func ValidateAgentConfig(cfg *AgentConfig) error {
	if cfg.Keepalived.VRIDBase <= 0 {
		return fmt.Errorf("keepalived.virtual-router-id-base must be greater than zero")
	}

	if cfg.Keepalived.Priority < 0 {
		return fmt.Errorf("keepalived.priority must be non-negative")
	}

	if cfg.Keepalived.Interface == "" {
		return fmt.Errorf("keepalived.interface must be set")
	}

	if cfg.Keepalived.OutputFile == "" {
		return fmt.Errorf("keepalived.output-file must be set")
	}

	if cfg.Nftables.OutputFile == "" {
		return fmt.Errorf("nftables.output-file must be set")
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
