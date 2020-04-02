package controller

import (
	"fmt"

	corelisters "k8s.io/client-go/listers/core/v1"

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
	endpoints corelisters.EndpointsLister) (LoadBalancerModelGenerator, error) {
	switch backendLayer {
	case config.BackendLayerNodePort:
		return NewNodePortLoadBalancerModelGenerator(
			l3portmanager, services, nodes,
		), nil
	case config.BackendLayerClusterIP:
		return NewClusterIPLoadBalancerModelGenerator(
			l3portmanager, services, nodes,
		), nil
	case config.BackendLayerPod:
		return NewPodLoadBalancerModelGenerator(
			l3portmanager, services, nodes, endpoints,
		), nil
	default:
		return nil, fmt.Errorf("invalid backend type: %q", backendLayer)
	}
}
