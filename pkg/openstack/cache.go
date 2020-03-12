package openstack

import (
	"time"

	"github.com/gophercloud/gophercloud"
	portsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/pagination"

	"k8s.io/klog"
)

type SimplePortCache struct {
	client     *gophercloud.ServiceClient
	opts       portsv2.ListOpts
	validUntil time.Time
	ttl        time.Duration
	ports      map[string]*portsv2.Port
}

type PortCache interface {
	GetPorts() ([]*portsv2.Port, error)
	GetPortByID(ID string) (*portsv2.Port, error)
	Invalidate()
}

func (client *OpenStackClient) NewPortCache(ttl time.Duration, opts portsv2.ListOpts) (*SimplePortCache, error) {
	networkingclient, err := client.NewNetworkV2()
	if err != nil {
		return nil, err
	}

	return NewPortCache(networkingclient, ttl, opts), nil
}

func NewPortCache(networkingclient *gophercloud.ServiceClient, ttl time.Duration, opts portsv2.ListOpts) *SimplePortCache {
	return &SimplePortCache{
		client: networkingclient,
		ttl:    ttl,
		opts:   opts,
	}
}

func (pc *SimplePortCache) GetPorts() ([]*portsv2.Port, error) {
	err := pc.refreshIfInvalid()
	if err != nil {
		return nil, err
	}

	result := make([]*portsv2.Port, len(pc.ports))
	i := 0
	for _, v := range pc.ports {
		result[i] = v
		i += 1
	}
	return result, nil
}

func (pc *SimplePortCache) GetPortByID(ID string) (*portsv2.Port, error) {
	err := pc.refreshIfInvalid()
	if err != nil {
		return nil, err
	}

	port, ok := pc.ports[ID]
	if !ok {
		return nil, nil
	}
	return port, nil
}

func (pc *SimplePortCache) refreshIfInvalid() error {
	now := time.Now()
	if now.Before(pc.validUntil) {
		klog.V(5).Infof("not refreshing port cache, it is still valid until %s", pc.validUntil)
		return nil
	}
	return pc.forceRefresh()
}

func (pc *SimplePortCache) forceRefresh() error {
	pager := portsv2.List(pc.client, pc.opts)
	entries := make(map[string]*portsv2.Port)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		ports, err := portsv2.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		for _, port := range ports {
			portCopy := &portsv2.Port{}
			*portCopy = port
			entries[port.ID] = portCopy
		}
		return true, nil
	})
	if err == nil {
		klog.V(5).Infof("successfully refreshed port cache. found %d ports", len(entries))
		pc.ports = entries
		pc.validUntil = time.Now().Add(pc.ttl)
	}
	return err
}

func (pc *SimplePortCache) Invalidate() {
	// set to zero time -> expired immediately
	pc.validUntil = time.Time{}
}
