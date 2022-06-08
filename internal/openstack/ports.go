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

type UncachedClient struct {
	client         *gophercloud.ServiceClient
	tag            string
	useFloatingIPs bool
}

type PortClient interface {
	GetPorts() ([]portsv2.Port, error)
	GetPortByID(ID string) (*portsv2.Port, *floatingipsv2.FloatingIP, error)
}

func NewPortClient(networkingclient *gophercloud.ServiceClient, tag string, useFloatingIPs bool) *UncachedClient {
	return &UncachedClient{
		client:         networkingclient,
		tag:            tag,
		useFloatingIPs: useFloatingIPs,
	}
}

func (pc *UncachedClient) GetPorts() (ports []portsv2.Port, err error) {
	err = portsv2.List(
		pc.client,
		portsv2.ListOpts{Tags: pc.tag},
	).EachPage(func(page pagination.Page) (bool, error) {
		fetched_ports, err := portsv2.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		for _, found_port := range fetched_ports {
			ports = append(ports, found_port)
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return ports, err
}

func (pc *UncachedClient) GetPortByID(ID string) (port *portsv2.Port, fip *floatingipsv2.FloatingIP, err error) {
	port, err = portsv2.Get(
		pc.client,
		ID,
	).Extract()
	if err != nil {
		return nil, nil, err
	}

	if pc.useFloatingIPs {
		err = floatingipsv2.List(
			pc.client,
			floatingipsv2.ListOpts{Tags: pc.tag, PortID: ID},
		).EachPage(func(page pagination.Page) (bool, error) {
			fips, err := floatingipsv2.ExtractFloatingIPs(page)
			if err != nil {
				return false, err
			}
			for _, found_fip := range fips {
				if fip != nil {
					// TODO: warn here?!
					klog.Warningf("Found multiple floating IPs for port %s (%s and %s at least)", ID, fip.ID, found_fip.ID)
				}
				fip = &found_fip
			}
			return true, nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return port, fip, nil
}
