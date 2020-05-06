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
package openstack

import (
	"time"

	"github.com/gophercloud/gophercloud"
	floatingipsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	portsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/pagination"

	"k8s.io/klog"
)

type CachedPort struct {
	Port       portsv2.Port
	FloatingIP *floatingipsv2.FloatingIP
}

type SimplePortCache struct {
	client         *gophercloud.ServiceClient
	tag            string
	useFloatingIPs bool
	validUntil     time.Time
	ttl            time.Duration
	ports          map[string]*CachedPort
}

type PortCache interface {
	GetPorts() ([]*portsv2.Port, error)
	GetPortByID(ID string) (*portsv2.Port, *floatingipsv2.FloatingIP, error)
	Invalidate()
}

func (client *OpenStackClient) NewPortCache(ttl time.Duration, tag string, useFloatingIPs bool) (*SimplePortCache, error) {
	networkingclient, err := client.NewNetworkV2()
	if err != nil {
		return nil, err
	}

	return NewPortCache(networkingclient, ttl, tag, useFloatingIPs), nil
}

func NewPortCache(networkingclient *gophercloud.ServiceClient, ttl time.Duration, tag string, useFloatingIPs bool) *SimplePortCache {
	return &SimplePortCache{
		client:         networkingclient,
		ttl:            ttl,
		tag:            tag,
		useFloatingIPs: useFloatingIPs,
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
		result[i] = &v.Port
		i += 1
	}
	return result, nil
}

func (pc *SimplePortCache) GetPortByID(ID string) (*portsv2.Port, *floatingipsv2.FloatingIP, error) {
	err := pc.refreshIfInvalid()
	if err != nil {
		return nil, nil, err
	}

	port, ok := pc.ports[ID]
	if !ok {
		return nil, nil, nil
	}
	return &port.Port, port.FloatingIP, nil
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
	// port ID -> FloatingIP, *NOT* floating IP ID -> FloatingIP
	var fipEntries map[string]*floatingipsv2.FloatingIP = nil
	if pc.useFloatingIPs {
		fipEntries = make(map[string]*floatingipsv2.FloatingIP)
		err := floatingipsv2.List(
			pc.client,
			floatingipsv2.ListOpts{Tags: pc.tag},
		).EachPage(func(page pagination.Page) (bool, error) {
			fips, err := floatingipsv2.ExtractFloatingIPs(page)
			if err != nil {
				return false, err
			}
			for _, fip := range fips {
				if fip.PortID == "" {
					continue
				}
				fipCopy := &floatingipsv2.FloatingIP{}
				*fipCopy = fip
				fipEntries[fip.PortID] = fipCopy
			}
			return true, nil
		})
		// if floating IPs are enabled, not being able to fetch them is actually
		// fatal for the refresh operation.
		if err != nil {
			return err
		}
	}

	entries := make(map[string]*CachedPort)
	err := portsv2.List(
		pc.client,
		portsv2.ListOpts{Tags: pc.tag},
	).EachPage(func(page pagination.Page) (bool, error) {
		ports, err := portsv2.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		for _, port := range ports {
			portEntry := &CachedPort{
				Port: port,
			}
			if fipEntries != nil {
				portEntry.FloatingIP = fipEntries[port.ID]
			}
			entries[port.ID] = portEntry
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
