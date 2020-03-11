package controller

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
)

var (
	ErrServiceNotMapped = errors.New("Service not mapped")
	ErrNoSuitablePort   = errors.New("No suitable port available")
)

const (
	AnnotationInboundPort = "cah-loadbalancer.k8s.cloudandheat.com/inbound-port"
)

type L3PortManager interface {
	ProvisionPort() (string, error)
	CleanUnusedPorts(usedPorts []string) error
}

type PortMapper interface {
	MapService(svc *corev1.Service) error
	UnmapService(svc *corev1.Service) error
	GetServiceL3Port(svc *corev1.Service) (string, error)
	GetLBConfiguration() error
	GetUsedL3Ports() ([]string, error)
}

type PortMapperImpl struct {
	l3manager L3PortManager
	services  map[string]model.ServiceModel
	l3ports   map[string]model.L3Port
}

func NewPortMapper(l3manager L3PortManager) PortMapper {
	return &PortMapperImpl{
		l3manager: l3manager,
		services:  make(map[string]model.ServiceModel),
		l3ports:   make(map[string]model.L3Port),
	}
}

func (c *PortMapperImpl) getServiceKey(svc *corev1.Service) string {
	return fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
}

func (c *PortMapperImpl) createNewL3Port() (string, error) {
	portID, err := c.l3manager.ProvisionPort()
	if err != nil {
		return "", err
	}
	c.emplaceL3Port(portID)
	return portID, nil
}

func (c *PortMapperImpl) emplaceL3Port(portID string) {
	c.l3ports[portID] = model.L3Port{
		Allocations: make(map[int32]string),
	}
}

func (c *PortMapperImpl) isPortSuitableFor(l3port model.L3Port, ports []model.L4Port) bool {
	for _, l4port := range ports {
		if !l3port.L4PortFree(l4port) {
			return false
		}
	}
	return true
}

func (c *PortMapperImpl) findL3PortFor(ports []model.L4Port) (string, error) {
	for portID, l3port := range c.l3ports {
		if c.isPortSuitableFor(l3port, ports) {
			return portID, nil
		}
	}

	return "", ErrNoSuitablePort
}

func (c *PortMapperImpl) MapService(svc *corev1.Service) error {
	var err error
	key := c.getServiceKey(svc)

	svcModel := model.ServiceModel{
		L3PortID: "",
		Ports:    make([]model.L4Port, len(svc.Spec.Ports)),
	}
	for i, k8sPort := range svc.Spec.Ports {
		svcModel.Ports[i] = model.L4Port{Protocol: k8sPort.Protocol, Port: k8sPort.Port}
	}

	portID := ""
	// first, see if the service has a preferred port
	if svc.Annotations != nil {
		portID, _ = svc.Annotations[AnnotationInboundPort]
		// yes, there is a preferred port
		// TODO: retrieve port information from backend to ensure that it really
		// exists!
		l3port, exists := c.l3ports[portID]
		if exists {
			// the port is already known and thus may have allocations. we have
			// to check if any allocations conflict
			if !c.isPortSuitableFor(l3port, svcModel.Ports) {
				// and they do! so we have to relocate the service to a
				// different port
				// TODO: it would be good if that caused an event on the Service
				klog.Warningf(
					"relocating service %q to a new port due to conflict on old port %s",
					key,
					portID)
				portID = ""
			}
		} else {
			c.emplaceL3Port(portID)
		}
	}

	// if the service did not give us a specific port to use, we have to look
	// further
	if portID == "" {
		// second, try to find an existing port with non-conflicting allocations
		portID, err = c.findL3PortFor(svcModel.Ports)
		if err == ErrNoSuitablePort {
			// if no existing port can fit the bill, we move on to create a new
			// port
			portID, err = c.createNewL3Port()
			if err != nil {
				// if that fails too, we simply cannot map the service.
				return err
			}
		} else if err != nil {
			return err
		}
	}

	svcModel.L3PortID = portID
	c.services[key] = svcModel
	l3port := c.l3ports[portID]
	for _, port := range svcModel.Ports {
		l3port.Allocations[port.Port] = key
	}

	return nil
}

func (c *PortMapperImpl) GetServiceL3Port(svc *corev1.Service) (string, error) {
	svcModel, ok := c.services[c.getServiceKey(svc)]
	if !ok {
		return "", ErrServiceNotMapped
	}
	return svcModel.L3PortID, nil
}

func (c *PortMapperImpl) GetLBConfiguration() error {
	return fmt.Errorf("Not implemented")
}

func (c *PortMapperImpl) GetUsedL3Ports() ([]string, error) {
	result := []string{}
	for id, l3port := range c.l3ports {
		if len(l3port.Allocations) == 0 {
			delete(c.l3ports, id)
			continue
		}
		result = append(result, id)
	}
	return result, nil
}

func (c *PortMapperImpl) UnmapService(svc *corev1.Service) error {
	key := c.getServiceKey(svc)
	delete(c.services, key)
	for _, l3port := range c.l3ports {
		for portNumber, user := range l3port.Allocations {
			if user == key {
				delete(l3port.Allocations, portNumber)
			}
		}
	}
	return nil
}
