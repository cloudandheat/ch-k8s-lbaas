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
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	ostesting "github.com/cloudandheat/ch-k8s-lbaas/internal/openstack/testing"
)

type portMapperFixture struct {
	l3portmanager *ostesting.MockL3PortManager
	portmapper    PortMapper
}

func newPortMapperFixture() *portMapperFixture {
	l3portmanager := ostesting.NewMockL3PortManager()

	l3portmanager.On("GetAvailablePorts").Return([]string{}, nil).Times(1)

	return &portMapperFixture{
		l3portmanager: l3portmanager,
		portmapper:    NewPortMapper(l3portmanager),
	}
}

func newPortMapperService(name string) *corev1.Service {
	svc := newService(name)
	svc.Spec.Ports = []corev1.ServicePort{
		corev1.ServicePort{
			Protocol: corev1.ProtocolTCP,
			Port:     80,
		},
		corev1.ServicePort{
			Protocol: corev1.ProtocolTCP,
			Port:     443,
		},
	}
	return svc
}

func TestGetServiceL3PortReturnsErrorForNonexistingService(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("foo")

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s))
	assert.Equal(t, "", portID)
	assert.NotNil(t, err)
	assert.Equal(t, err, ErrServiceNotMapped)
}

func TestMapFirstUnknownServiceAssignsNewPort(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service")

	f.l3portmanager.On("ProvisionPort").Return("port-id", nil)

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s))
	assert.Nil(t, err)
	assert.Equal(t, "port-id", portID)
}

func TestMapServicePropagatesPortAllocationError(t *testing.T) {
	provisionError := errors.New("some error")
	f := newPortMapperFixture()
	s := newPortMapperService("test-service")

	f.l3portmanager.On("ProvisionPort").Return("dummy", provisionError)

	err := f.portmapper.MapService(s)
	assert.Equal(t, err, provisionError)
}

func TestPortAllocationErrorLeavesServiceUnmapped(t *testing.T) {
	provisionError := errors.New("some error")
	f := newPortMapperFixture()
	s := newPortMapperService("test-service")

	f.l3portmanager.On("ProvisionPort").Return("dummy", provisionError)

	err := f.portmapper.MapService(s)
	assert.Equal(t, err, provisionError)

	_, err = f.portmapper.GetServiceL3Port(model.FromService(s))
	assert.Equal(t, err, ErrServiceNotMapped)
}

func TestMapServiceWithNonConflictingL4PortsReusesL3Port(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")
	s2.Spec.Ports = []corev1.ServicePort{
		corev1.ServicePort{
			Protocol: corev1.ProtocolUDP,
			Port:     53,
		},
	}

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("", fmt.Errorf("no more ports"))

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)
}

func TestMapServiceWithConflictingL4PortsAllocatesNewL3Port(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)
}

func TestRemappingTheSameServiceDoesNotChangePorts(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)
	setPortAnnotation(s1, portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)

	err = f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)
}

func TestRemappingTheSameServiceWithoutAnnotationIsHandledGracefully(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-3", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)

	err = f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	portIDs, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(portIDs))
	assert.Contains(t, portIDs, "port-id-2")
	assert.Contains(t, portIDs, "port-id-1")
}

func TestRemappingAnUpdatedServiceWithAnnotationDoesNotAllocateANewPort(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	setPortAnnotation(s1, portID)
	s1.Spec.Ports = append(s1.Spec.Ports, corev1.ServicePort{Protocol: corev1.ProtocolUDP, Port: 53})

	err = f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)
}

func TestRemappingAnUpdatedServiceWithoutAnnotationDoesNotAllocateANewPort(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	s1.Spec.Ports = append(s1.Spec.Ports, corev1.ServicePort{Protocol: corev1.ProtocolUDP, Port: 53})

	err = f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)
}

func TestRemappingAnUpdatedServiceWithStaleAnnotationDoesNotAllocateANewPort(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	setPortAnnotation(s1, "old-port-id")
	s1.Spec.Ports = append(s1.Spec.Ports, corev1.ServicePort{Protocol: corev1.ProtocolUDP, Port: 53})

	err = f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)
}

func TestRemappingAnUpdatedServiceWithAnnotationClearsOldAllocations(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	setPortAnnotation(s1, portID)
	s1.Spec.Ports = []corev1.ServicePort{{Protocol: corev1.ProtocolUDP, Port: 53}}

	err = f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)
}

func TestUnmapServiceRemovesPortAssignment(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service-1")
	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	err = f.portmapper.UnmapService(model.FromService(s))
	assert.Nil(t, err)

	_, err = f.portmapper.GetServiceL3Port(model.FromService(s))
	assert.Equal(t, err, ErrServiceNotMapped)
}

func TestUnmapServiceRemovesPortAllocations(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	err = f.portmapper.UnmapService(model.FromService(s1))
	assert.Nil(t, err)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)
}

func TestGetUsedL3PortsIsEmptyByDefault(t *testing.T) {
	f := newPortMapperFixture()

	ports, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, ports, []string{})
}

func TestGetUsedL3PortsContainsMappedPorts(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	ports, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, ports, []string{"port-id-1"})

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	ports, err = f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, len(ports), 2)
	assert.Contains(t, ports, "port-id-1")
	assert.Contains(t, ports, "port-id-2")
}

func TestGetUsedL3PortsDoesNotReturnUnusedPorts(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	ports, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, ports, []string{"port-id-1"})

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	ports, err = f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, len(ports), 2)
	assert.Contains(t, ports, "port-id-1")
	assert.Contains(t, ports, "port-id-2")

	err = f.portmapper.UnmapService(model.FromService(s1))
	assert.Nil(t, err)

	ports, err = f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, ports, []string{"port-id-2"})

	err = f.portmapper.UnmapService(model.FromService(s2))
	assert.Nil(t, err)

	ports, err = f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, ports, []string{})
}

func TestGetUsedL3PortsCleansUpUnusedPorts(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	err = f.portmapper.UnmapService(model.FromService(s1))
	assert.Nil(t, err)

	f.portmapper.GetUsedL3Ports()

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)
}

func TestUnmapServiceWithUnknownServiceReturnsNil(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service-1")

	err := f.portmapper.UnmapService(model.FromService(s))
	assert.Nil(t, err)
}

func TestMapServiceWithAnnotationInjectsThePortWithoutAllocation(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service-1")
	s.Annotations = make(map[string]string)
	s.Annotations[AnnotationInboundPort] = "port-id-x"

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-x", portID)
}

func TestMapServiceWithAnnotationIsMovedToAnotherPortOnConflict(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")

	s2 := newPortMapperService("test-service-2")
	s2.Annotations = make(map[string]string)
	s2.Annotations[AnnotationInboundPort] = "port-id-1"

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)
}

func TestSetAvailableL3PortsWithSameSetOfPortsHasNoVisibleEffect(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service")

	f.l3portmanager.On("ProvisionPort").Return("port-id", nil)

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	evicted, err := f.portmapper.SetAvailableL3Ports([]string{"port-id"})
	assert.Nil(t, err)
	assert.Equal(t, []model.ServiceIdentifier{}, evicted)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s))
	assert.Nil(t, err)
	assert.Equal(t, "port-id", portID)

	portIDs, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, []string{portID}, portIDs)
}

func TestSetAvailableL3PortsWithMorePortsHasNoVisibleEffect(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service")

	f.l3portmanager.On("ProvisionPort").Return("port-id", nil)

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	evicted, err := f.portmapper.SetAvailableL3Ports([]string{"some-port", "port-id"})
	assert.Nil(t, err)
	assert.Equal(t, []model.ServiceIdentifier{}, evicted)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s))
	assert.Nil(t, err)
	assert.Equal(t, "port-id", portID)

	portIDs, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, []string{portID}, portIDs)
}

func TestSetAvailableL3PortsWithMissingPortEvictsService(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service")

	f.l3portmanager.On("ProvisionPort").Return("port-id", nil)

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	evicted, err := f.portmapper.SetAvailableL3Ports([]string{})
	assert.Nil(t, err)
	assert.Equal(t, []model.ServiceIdentifier{model.FromService(s)}, evicted)

	_, err = f.portmapper.GetServiceL3Port(model.FromService(s))
	assert.Equal(t, ErrServiceNotMapped, err)
}

func TestSetAvailableL3PortsWithMissingPortRemovesPort(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service")

	f.l3portmanager.On("ProvisionPort").Return("port-id", nil)

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	evicted, err := f.portmapper.SetAvailableL3Ports([]string{})
	assert.Nil(t, err)
	assert.Equal(t, []model.ServiceIdentifier{model.FromService(s)}, evicted)

	portIDs, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, []string{}, portIDs)
}

func TestSetAvailableL3PortsWithMissingPortOnlyEvictsAffectedService(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	evicted, err := f.portmapper.SetAvailableL3Ports([]string{"port-id-2"})
	assert.Nil(t, err)
	assert.Equal(t, []model.ServiceIdentifier{model.FromService(s1)}, evicted)

	_, err = f.portmapper.GetServiceL3Port(model.FromService(s1))
	assert.Equal(t, ErrServiceNotMapped, err)

	portID, err := f.portmapper.GetServiceL3Port(model.FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)

	portIDs, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, []string{"port-id-2"}, portIDs)
}

func TestMapServiceMakesServiceAppearInModel(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s1i := model.FromService(s1)
	s1k := s1i.ToKey()
	s2 := newPortMapperService("test-service-2")
	s2i := model.FromService(s2)
	s2k := s2i.ToKey()

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)
	f.l3portmanager.On("ProvisionPort").Return("port-id-2", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	pmmodel := f.portmapper.GetModel()
	assert.Contains(t, pmmodel, s1k)
	assert.Contains(t, pmmodel, s2k)

	s1p, err := f.portmapper.GetServiceL3Port(s1i)
	assert.Nil(t, err)

	s2p, err := f.portmapper.GetServiceL3Port(s2i)
	assert.Nil(t, err)

	port, ok := pmmodel[s1k]
	assert.True(t, ok)
	assert.Equal(t, s1p, port)

	port, ok = pmmodel[s2k]
	assert.True(t, ok)
	assert.Equal(t, s2p, port)
}
