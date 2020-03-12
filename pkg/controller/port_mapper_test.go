package controller

import (
	"errors"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"

	ostesting "github.com/cloudandheat/cah-loadbalancer/pkg/openstack/testing"
)

type portMapperFixture struct {
	l3portmanager *ostesting.MockL3PortManager
	portmapper    PortMapper
}

func newPortMapperFixture() *portMapperFixture {
	l3portmanager := ostesting.NewMockL3PortManager()
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

	portID, err := f.portmapper.GetServiceL3Port(FromService(s))
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

	portID, err := f.portmapper.GetServiceL3Port(FromService(s))
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

	_, err = f.portmapper.GetServiceL3Port(FromService(s))
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

	portID, err := f.portmapper.GetServiceL3Port(FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(FromService(s2))
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

	portID, err := f.portmapper.GetServiceL3Port(FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)
}

func TestUnmapServiceRemovesPortAssignment(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service-1")
	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	err = f.portmapper.UnmapService(FromService(s))
	assert.Nil(t, err)

	_, err = f.portmapper.GetServiceL3Port(FromService(s))
	assert.Equal(t, err, ErrServiceNotMapped)
}

func TestUnmapServiceRemovesPortAllocations(t *testing.T) {
	f := newPortMapperFixture()
	s1 := newPortMapperService("test-service-1")
	s2 := newPortMapperService("test-service-2")

	f.l3portmanager.On("ProvisionPort").Return("port-id-1", nil).Times(1)

	err := f.portmapper.MapService(s1)
	assert.Nil(t, err)

	err = f.portmapper.UnmapService(FromService(s1))
	assert.Nil(t, err)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(FromService(s2))
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

	err = f.portmapper.UnmapService(FromService(s1))
	assert.Nil(t, err)

	ports, err = f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, ports, []string{"port-id-2"})

	err = f.portmapper.UnmapService(FromService(s2))
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

	err = f.portmapper.UnmapService(FromService(s1))
	assert.Nil(t, err)

	f.portmapper.GetUsedL3Ports()

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)
}

func TestUnmapServiceWithUnknownServiceReturnsNil(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service-1")

	err := f.portmapper.UnmapService(FromService(s))
	assert.Nil(t, err)
}

func TestMapServiceWithAnnotationInjectsThePortWithoutAllocation(t *testing.T) {
	f := newPortMapperFixture()
	s := newPortMapperService("test-service-1")
	s.Annotations = make(map[string]string)
	s.Annotations[AnnotationInboundPort] = "port-id-x"

	err := f.portmapper.MapService(s)
	assert.Nil(t, err)

	portID, err := f.portmapper.GetServiceL3Port(FromService(s))
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

	portID, err := f.portmapper.GetServiceL3Port(FromService(s1))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-1", portID)

	err = f.portmapper.MapService(s2)
	assert.Nil(t, err)

	portID, err = f.portmapper.GetServiceL3Port(FromService(s2))
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
	assert.Equal(t, []ServiceIdentifier{}, evicted)

	portID, err := f.portmapper.GetServiceL3Port(FromService(s))
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
	assert.Equal(t, []ServiceIdentifier{}, evicted)

	portID, err := f.portmapper.GetServiceL3Port(FromService(s))
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
	assert.Equal(t, []ServiceIdentifier{FromService(s)}, evicted)

	_, err = f.portmapper.GetServiceL3Port(FromService(s))
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
	assert.Equal(t, []ServiceIdentifier{FromService(s)}, evicted)

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
	assert.Equal(t, []ServiceIdentifier{FromService(s1)}, evicted)

	_, err = f.portmapper.GetServiceL3Port(FromService(s1))
	assert.Equal(t, ErrServiceNotMapped, err)

	portID, err := f.portmapper.GetServiceL3Port(FromService(s2))
	assert.Nil(t, err)
	assert.Equal(t, "port-id-2", portID)

	portIDs, err := f.portmapper.GetUsedL3Ports()
	assert.Nil(t, err)
	assert.Equal(t, []string{"port-id-2"}, portIDs)
}
