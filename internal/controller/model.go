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
	"fmt"

	corelisters "k8s.io/client-go/listers/core/v1"
	networkinglisters "k8s.io/client-go/listers/networking/v1"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

type LoadBalancerModelGenerator interface {
	GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error)
}

func NewLoadBalancerModelGenerator(
	backendLayer config.BackendLayer,
	l3portmanager openstack.L3PortManager,
	services corelisters.ServiceLister,
	nodes corelisters.NodeLister,
	endpoints corelisters.EndpointsLister,
	networkpolicies networkinglisters.NetworkPolicyLister,
	pods corelisters.PodLister) (LoadBalancerModelGenerator, error) {
	switch backendLayer {
	case config.BackendLayerNodePort:
		return NewNodePortLoadBalancerModelGenerator(
			l3portmanager, services, nodes,
		), nil
	case config.BackendLayerClusterIP:
		return NewClusterIPLoadBalancerModelGenerator(
			l3portmanager, services,
		), nil
	case config.BackendLayerPod:
		return NewPodLoadBalancerModelGenerator(
			l3portmanager, services, endpoints, networkpolicies, pods,
		), nil
	default:
		return nil, fmt.Errorf("invalid backend type: %q", backendLayer)
	}
}
