package controller

import (
	"fmt"

	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

type ClusterIPLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
	nodes         corelisters.NodeLister
}

func NewClusterIPLoadBalancerModelGenerator(
	l3portmanager openstack.L3PortManager,
	services corelisters.ServiceLister,
	nodes corelisters.NodeLister) *ClusterIPLoadBalancerModelGenerator {
	return &ClusterIPLoadBalancerModelGenerator{
		l3portmanager: l3portmanager,
		services:      services,
		nodes:         nodes,
	}
}

func (g *ClusterIPLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
	return nil, fmt.Errorf("not implemented")
}
