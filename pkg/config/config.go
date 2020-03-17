package config

import (
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
	VRIDBase int `toml:"virtual-router-id-base"`
	Interface string `toml:"interface"`
	OutputFile string `toml:"output-file"`
}

type Nftables struct {
	FilterTableName         string `toml:"filter-table-name"`
	FilterTableType         string `toml:"filter-table-type"`
	FilterForwardChainName  string `toml:"filter-forward-chain"`
	NATTableName            string `toml:"nat-table-name"`
	NATPreroutingChainName  string `toml:"nat-prerouting-chain"`
	NATPostroutingChainName string `toml:"nat-postrouting-chain"`
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

func FillAgentConfig(cfg *AgentConfig) {
	defaultString(&cfg.Keepalived.VRRPPassword, "useless")

	defaultString(&cfg.Nftables.FilterTableName, "filter")
	defaultString(&cfg.Nftables.FilterTableType, "inet")
	defaultString(&cfg.Nftables.FilterForwardChainName, "forward")
	defaultString(&cfg.Nftables.NATTableName, "nat")
	defaultString(&cfg.Nftables.NATPreroutingChainName, "prerouting")
	defaultString(&cfg.Nftables.NATPostroutingChainName, "postrouting")
}
