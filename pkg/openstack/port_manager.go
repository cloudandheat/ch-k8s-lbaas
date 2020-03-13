package openstack

import (
	"time"

	"github.com/gophercloud/gophercloud"
	tags "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/attributestags"
	floatingipsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	portsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	subnetsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/pagination"

	"k8s.io/klog"
)

const (
	TagLBManagedPort         = "cah-loadbalancer.k8s.cloudandheat.com/managed"
	DescriptionLBManagedPort = "Managed by cah-loadbalancer"
)

// We need options which are not included in the default gophercloud struct
type CustomCreateOpts struct {
	NetworkID           string                `json:"network_id" required:"true"`
	Name                string                `json:"name,omitempty"`
	Description         string                `json:"description,omitempty"`
	AdminStateUp        *bool                 `json:"admin_state_up,omitempty"`
	MACAddress          string                `json:"mac_address,omitempty"`
	FixedIPs            interface{}           `json:"fixed_ips,omitempty"`
	DeviceID            string                `json:"device_id,omitempty"`
	DeviceOwner         string                `json:"device_owner,omitempty"`
	TenantID            string                `json:"tenant_id,omitempty"`
	ProjectID           string                `json:"project_id,omitempty"`
	SecurityGroups      *[]string             `json:"security_groups,omitempty"`
	AllowedAddressPairs []portsv2.AddressPair `json:"allowed_address_pairs,omitempty"`

	// specifically this one
	PortSecurityEnabled *bool `json:"port_security_enabled",omitempty"`
}

func (opts CustomCreateOpts) ToPortCreateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "port")
}

type L3PortManager interface {
	ProvisionPort() (string, error)
	CleanUnusedPorts(usedPorts []string) error
	GetAvailablePorts() ([]string, error)
}

type OpenStackL3PortManager struct {
	client    *gophercloud.ServiceClient
	networkID string
	cfg       *NetworkingOpts
	cache     PortCache
}

func (client *OpenStackClient) NewOpenStackL3PortManager(networkConfig *NetworkingOpts) (*OpenStackL3PortManager, error) {
	networkingclient, err := client.NewNetworkV2()
	if err != nil {
		return nil, err
	}

	subnet, err := subnetsv2.Get(networkingclient, networkConfig.SubnetID).Extract()
	if err != nil {
		return nil, err
	}

	networkID := subnet.NetworkID

	return &OpenStackL3PortManager{
		client:    networkingclient,
		cfg:       networkConfig,
		networkID: networkID,
		cache: NewPortCache(
			networkingclient,
			30*time.Second,
			TagLBManagedPort,
			networkConfig.UseFloatingIPs,
		),
	}, nil
}

func (pm *OpenStackL3PortManager) provisionFloatingIP(portID string) error {
	fip, err := floatingipsv2.Create(
		pm.client,
		floatingipsv2.CreateOpts{
			Description:       DescriptionLBManagedPort,
			FloatingNetworkID: pm.cfg.FloatingIPNetworkID,
			PortID:            portID,
		},
	).Extract()

	if err != nil {
		return err
	}

	cleanupFip := func() {
		deleteErr := floatingipsv2.Delete(pm.client, fip.ID).ExtractErr()
		if deleteErr != nil {
			klog.Warningf(
				"resource leak: could not delete dysfunctional floating IP %q: %s:",
				fip.ID,
				deleteErr)
		}
	}

	_, err = tags.ReplaceAll(pm.client, "floatingips", fip.ID, tags.ReplaceAllOpts{
		Tags: []string{TagLBManagedPort},
	}).Extract()

	if err != nil {
		cleanupFip()
		return err
	}

	return nil
}

func boolPtr(v bool) *bool {
	return &v
}

func (pm *OpenStackL3PortManager) ProvisionPort() (string, error) {
	port, err := portsv2.Create(
		pm.client,
		CustomCreateOpts{
			NetworkID:   pm.networkID,
			Description: DescriptionLBManagedPort,
			FixedIPs: []portsv2.IP{
				{SubnetID: pm.cfg.SubnetID},
			},
			PortSecurityEnabled: boolPtr(false),
		},
	).Extract()
	// XXX: this is meh because we can only set the tag after the port was
	// created. If we get killed between the previous line and setting the
	// tag, the port will linger, unusedly.
	// If this is a problem, we’ll have to switch to matching based on the name
	// or description instead.
	if err != nil {
		return "", err
	}

	cleanupPort := func() {
		deleteErr := portsv2.Delete(pm.client, port.ID).ExtractErr()
		if deleteErr != nil {
			klog.Warningf(
				"resource leak: could not delete dysfunctional port %q: %s:",
				port.ID,
				deleteErr)
		}
	}

	_, err = tags.ReplaceAll(pm.client, "ports", port.ID, tags.ReplaceAllOpts{
		Tags: []string{TagLBManagedPort},
	}).Extract()

	if err != nil {
		cleanupPort()
		return "", err
	}

	if pm.cfg.UseFloatingIPs {
		err = pm.provisionFloatingIP(port.ID)
		if err != nil {
			cleanupPort()
			return "", nil
		}
	}

	pm.cache.Invalidate()
	return port.ID, nil
}

func (pm *OpenStackL3PortManager) deleteUnusedFloatingIPs() error {
	pager := floatingipsv2.List(
		pm.client,
		floatingipsv2.ListOpts{
			Tags: TagLBManagedPort,
		},
	)

	toDelete := make([]string, 0)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		fips, err := floatingipsv2.ExtractFloatingIPs(page)
		if err != nil {
			return false, err
		}
		for _, fip := range fips {
			if fip.PortID == "" {
				// no assigned port, delete
				toDelete = append(toDelete, fip.ID)
			}
		}
		return true, nil
	})

	// even in case of an error, we can at least try to delete the fips we
	// already gathered
	for _, fipID := range toDelete {
		deleteErr := floatingipsv2.Delete(pm.client, fipID).ExtractErr()
		if deleteErr != nil {
			klog.Warningf(
				"Failed to delete orphaned floating ip %q: %s. The operation will be retried later.",
				fipID,
				deleteErr.Error())
		}
	}

	return err
}

func (pm *OpenStackL3PortManager) CleanUnusedPorts(usedPorts []string) error {
	ports, err := pm.cache.GetPorts()
	if err != nil {
		return err
	}

	usedPortsMap := make(map[string]bool)
	for _, portID := range usedPorts {
		usedPortsMap[portID] = true
	}

	anyDeleted := false
	for _, port := range ports {
		if _, inUse := usedPortsMap[port.ID]; inUse {
			continue
		}

		// port not in use, issue deletion
		err := portsv2.Delete(pm.client, port.ID).ExtractErr()
		if err != nil {
			klog.Warningf("Failed to delete unused port %q: %s. The operation will be retried later.", port.ID, err)
		}
		anyDeleted = true
	}

	if anyDeleted {
		pm.cache.Invalidate()
		return pm.deleteUnusedFloatingIPs()
	}
	return nil
}

func (pm *OpenStackL3PortManager) GetAvailablePorts() ([]string, error) {
	ports, err := pm.cache.GetPorts()
	if err != nil {
		return nil, err
	}

	result := make([]string, len(ports))
	i := 0
	for _, port := range ports {
		result[i] = port.ID
		i += 1
	}
	return result, nil
}
