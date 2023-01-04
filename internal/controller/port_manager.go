package controller

type L3PortManager interface {
	// ProvisionPort Create a new L3 port and return its id
	ProvisionPort() (string, error)
	// CleanUnusedPorts Delete all L3 ports that are currently not used
	CleanUnusedPorts(usedPorts []string) error
	// GetAvailablePorts Return all L3 ports that are available
	GetAvailablePorts() ([]string, error)
	// GetExternalAddress For a given portID, return the external address (floating IP) and hostname
	GetExternalAddress(portID string) (string, string, error)
	// GetInternalAddress For a given portID, return the internal address (target of the floating IP)
	GetInternalAddress(portID string) (string, error)
}
