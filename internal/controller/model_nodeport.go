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
package controller

import (
	"errors"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

var (
	ErrInvalidIpAddress = errors.New("the string is not a valid textual representation of an IP address")
)

type NodePortLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
	nodes         corelisters.NodeLister
}

func NewNodePortLoadBalancerModelGenerator(
	l3portmanager openstack.L3PortManager,
	services corelisters.ServiceLister,
	nodes corelisters.NodeLister) *NodePortLoadBalancerModelGenerator {
	return &NodePortLoadBalancerModelGenerator{
		l3portmanager: l3portmanager,
		services:      services,
		nodes:         nodes,
	}
}

func validateIpAddress(ipString string) error {
	ipParsed := net.ParseIP(ipString)
	if ipParsed != nil {
		return nil
	}
	return ErrInvalidIpAddress
}

func isIPv4Address(ipString string) bool {
	return strings.Count(ipString, ":") == 0 && strings.Count(ipString, ".") == 3
}

func isIPv6Address(ipString string) bool {
	return strings.Count(ipString, ":") >= 2
}

func (g *NodePortLoadBalancerModelGenerator) getDestinationAddresses() (addressesV4 []string, addressesV6 []string, err error) {
	nodes, err := g.nodes.List(labels.Everything())
	if err != nil {
		return nil, nil, err
	}

	for _, node := range nodes {
		for _, addr := range node.Status.Addresses {
			if addr.Type != corev1.NodeInternalIP {
				continue
			}
			if validateIpAddress(addr.Address) != nil {
				continue
			}
			if isIPv4Address(addr.Address) {
				addressesV4 = append(addressesV4, addr.Address)
			} else if isIPv6Address(addr.Address) {
				addressesV6 = append(addressesV6, addr.Address)
			} else {
				continue
			}
		}
	}

	return addressesV4, addressesV6, nil
}

func (g *NodePortLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
	addressesV4, addressesV6, err := g.getDestinationAddresses()
	if err != nil {
		return nil, err
	}

	result := &model.LoadBalancer{}

	ingressMap := map[string]model.IngressIP{}

	for serviceKey, portID := range portAssignment {
		id, _ := model.FromKey(serviceKey)
		svc, err := g.services.Services(id.Namespace).Get(id.Name)
		if err != nil {
			return nil, err
		}

		ingress, ok := ingressMap[portID]
		if !ok {
			ingressIP, err := g.l3portmanager.GetInternalAddress(portID)
			if err != nil {
				return nil, err
			}
			ingress = model.IngressIP{
				Address: ingressIP,
				Ports:   []model.PortForward{},
			}
		}

		if validateIpAddress(ingress.Address) != nil {
			continue
		}

		var destAddresses []string

		if isIPv4Address(ingress.Address) {
			destAddresses = append(destAddresses, addressesV4...)
		} else if isIPv6Address(ingress.Address) {
			destAddresses = append(destAddresses, addressesV6...)
		} else {
			klog.Warningf(
				"could not determine address family of ingress IP %q for service %q",
				ingress.Address,
				svc.Name)
			continue
		}

		for _, svcPort := range svc.Spec.Ports {
			ingress.Ports = append(ingress.Ports, model.PortForward{
				Protocol:             svcPort.Protocol,
				InboundPort:          svcPort.Port,
				DestinationPort:      svcPort.NodePort,
				DestinationAddresses: destAddresses,
			})
		}

		ingressMap[portID] = ingress
	}

	result.Ingress = make([]model.IngressIP, len(ingressMap))
	i := 0
	for _, ingress := range ingressMap {
		result.Ingress[i] = ingress
		i++
	}

	return result, nil
}
