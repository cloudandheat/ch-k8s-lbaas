package controller

type L3PortManager interface {
	// ProvisionPort creates a new L3 port and returns its id
	ProvisionPort() (string, error)
	// CleanUnusedPorts deletes all L3 ports that are currently not used
	CleanUnusedPorts(usedPorts []string) error
	// GetAvailablePorts returns all L3 ports that are available
	GetAvailablePorts() ([]string, error)
	// GetExternalAddress returns the external address (floating IP) and hostname for a given portID
	GetExternalAddress(portID string) (string, string, error)
	// GetInternalAddress returns the internal address (target of the floating IP) for a given portID,
	GetInternalAddress(portID string) (string, error)
	// CheckPortExists checks if there exists a port for the given portID
	CheckPortExists(portID string) (bool, error)
}
