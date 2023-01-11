package static

import (
	"fmt"
	"net/netip"
)

type Config struct {
	IPv4Addresses []netip.Addr `toml:"ipv4-addresses"`
	// IPv6Addresses []netip.Addr `toml:"ipv6-addresses"`
}

type StaticL3PortManager struct {
	cfg *Config
}

func NewStaticL3PortManager(config *Config) (*StaticL3PortManager, error) {
	return &StaticL3PortManager{
		cfg: config,
	}, nil
}

func (pm *StaticL3PortManager) ProvisionPort() (string, error) {
	return "", fmt.Errorf("cannot provision new ports when using static port manager")
}

func (pm *StaticL3PortManager) CleanUnusedPorts(usedPorts []string) error {
	return nil
}

func (pm *StaticL3PortManager) GetAvailablePorts() ([]string, error) {
	ports := make([]string, len(pm.cfg.IPv4Addresses))

	for _, addr := range pm.cfg.IPv4Addresses {
		ports = append(ports, addr.String())
	}

	return ports, nil
}

func (pm *StaticL3PortManager) GetExternalAddress(portID string) (string, string, error) {
	return portID, "", nil
}

func (pm *StaticL3PortManager) GetInternalAddress(portID string) (string, error) {
	return portID, nil
}
