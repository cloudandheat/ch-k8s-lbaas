package controller

import (
	goerrors "errors"

	corev1 "k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"k8s.io/klog"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

var (
	errPortNotFoundInSubset = goerrors.New("port not found in subset")
)

type PodLoadBalancerModelGenerator struct {
	l3portmanager openstack.L3PortManager
	services      corelisters.ServiceLister
	endpoints     corelisters.EndpointsLister
}

func NewPodLoadBalancerModelGenerator(
	l3portmanager openstack.L3PortManager,
	services corelisters.ServiceLister,
	endpoints corelisters.EndpointsLister) *PodLoadBalancerModelGenerator {
	return &PodLoadBalancerModelGenerator{
		l3portmanager: l3portmanager,
		services:      services,
		endpoints:     endpoints,
	}
}

func (g *PodLoadBalancerModelGenerator) findPort(subset *corev1.EndpointSubset, name string, targetPort int32, protocol corev1.Protocol) (int32, error) {
	nameMatch := int32(-1)
	portMatch := int32(-1)
	for _, epPort := range subset.Ports {
		if epPort.Protocol != protocol {
			continue
		}
		if name != "" && epPort.Name == name {
			nameMatch = epPort.Port
		}
		if epPort.Port == targetPort {
			portMatch = epPort.Port
		}
	}

	if nameMatch >= 0 {
		return nameMatch, nil
	}
	if portMatch >= 0 {
		return portMatch, nil
	}
	return -1, errPortNotFoundInSubset
}

func (g *PodLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
	result := &model.LoadBalancer{}

	ingressMap := map[string]model.IngressIP{}

	for serviceKey, portID := range portAssignment {
		id, _ := model.FromKey(serviceKey)
		svc, err := g.services.Services(id.Namespace).Get(id.Name)
		if err != nil {
			return nil, err
		}

		ep, err := g.endpoints.Endpoints(id.Namespace).Get(id.Name)
		if err != nil {
			// no endpoints exist or are not retrievable -> we ignore that for
			// now because this may happen during bootstrapping of a service
			continue
		}
		if len(ep.Subsets) < 1 {
			// no point in doing anything with the service here
			continue
		}
		// TODO: handle multiple subsets. This is tricky because our model
		// currently does not support different ports per destination IP.
		epSubset := ep.Subsets[0]
		if len(ep.Subsets) > 1 {
			klog.Warningf(
				"LB model for service %s will be inaccurate: more than one subset",
				serviceKey,
			)
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
			targetPort := int32(svcPort.TargetPort.IntValue())
			portName := svcPort.Name
			if targetPort == 0 {
				targetPort = svcPort.Port
				portName = svcPort.TargetPort.String()
			}
			destinationPort, err := g.findPort(
				&epSubset,
				portName, targetPort, svcPort.Protocol,
			)
			if err != nil {
				klog.Warningf(
					"LB model for service %s is inaccurate: failed to find matching Endpoints for Service Port %#v",
					serviceKey,
					svcPort,
				)
				continue
			}

			addresses := make([]string, len(epSubset.Addresses))
			for i, addr := range epSubset.Addresses {
				addresses[i] = addr.IP
			}

			ingress.Ports = append(ingress.Ports, model.PortForward{
				Protocol:             svcPort.Protocol,
				InboundPort:          svcPort.Port,
				DestinationPort:      destinationPort,
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
