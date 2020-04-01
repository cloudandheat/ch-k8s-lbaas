package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

type NodePortLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
	nodes         corelisters.NodeLister
}

type ClusterIPLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
	nodes         corelisters.NodeLister
}

type PodLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
	nodes         corelisters.NodeLister
	endpoints     corelisters.EndpointsLister
}

type LoadBalancerModelGenerator interface {
	GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error)
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

func (g *NodePortLoadBalancerModelGenerator) getDestinationAddresses() (result []string, err error) {
	nodes, err := g.nodes.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result = []string{}
	for _, node := range nodes {
		for _, addr := range node.Status.Addresses {
			if addr.Type != corev1.NodeInternalIP {
				continue
			}
			result = append(result, addr.Address)
		}
	}

	return result, nil
}

func (g *NodePortLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
	addresses, err := g.getDestinationAddresses()
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

		for _, svcPort := range svc.Spec.Ports {
			ingress.Ports = append(ingress.Ports, model.PortForward{
				Protocol:             svcPort.Protocol,
				InboundPort:          svcPort.Port,
				DestinationPort:      svcPort.NodePort,
				DestinationAddresses: addresses,
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

func (g *ClusterIPLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
	return nil, fmt.Errorf("not implemented")
}

func (g *PodLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
	return nil, fmt.Errorf("not implemented")
}
