package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

type DefaultLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
	nodes         corelisters.NodeLister
}

type LoadBalancerModelGenerator interface {
	GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error)
}

func NewDefaultLoadBalancerModelGenerator(
	l3portmanager openstack.L3PortManager,
	services corelisters.ServiceLister,
	nodes corelisters.NodeLister) *DefaultLoadBalancerModelGenerator {
	return &DefaultLoadBalancerModelGenerator{
		l3portmanager: l3portmanager,
		services:      services,
		nodes:         nodes,
	}
}

func (g *DefaultLoadBalancerModelGenerator) getDestinationAddresses() (result []string, err error) {
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

func (g *DefaultLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
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
