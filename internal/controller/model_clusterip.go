package controller

import (
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

type ClusterIPLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
}

func NewClusterIPLoadBalancerModelGenerator(
	l3portmanager openstack.L3PortManager,
	services corelisters.ServiceLister) *ClusterIPLoadBalancerModelGenerator {
	return &ClusterIPLoadBalancerModelGenerator{
		l3portmanager: l3portmanager,
		services:      services,
	}
}

func (g *ClusterIPLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
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
				DestinationPort:      svcPort.Port,
				DestinationAddresses: []string{svc.Spec.ClusterIP},
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
