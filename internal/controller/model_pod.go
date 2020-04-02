package controller

import (
	"fmt"

	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

type PodLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
	nodes         corelisters.NodeLister
	endpoints     corelisters.EndpointsLister
}

func NewPodLoadBalancerModelGenerator(
	l3portmanager openstack.L3PortManager,
	services corelisters.ServiceLister,
	nodes corelisters.NodeLister,
	endpoints corelisters.EndpointsLister) *PodLoadBalancerModelGenerator {
	return &PodLoadBalancerModelGenerator{
		l3portmanager: l3portmanager,
		services:      services,
		nodes:         nodes,
		endpoints:     endpoints,
	}
}

func (g *PodLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
	return nil, fmt.Errorf("not implemented")
}
