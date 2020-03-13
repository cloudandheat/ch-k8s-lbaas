package controller

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"

	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
	ostesting "github.com/cloudandheat/cah-loadbalancer/pkg/openstack/testing"
)

type generatorFixture struct {
	t *testing.T

	l3portmanager *ostesting.MockL3PortManager

	kubeclient    *k8sfake.Clientset
	serviceLister []*corev1.Service
	nodeLister    []*corev1.Node
	kubeobjects   []runtime.Object
}

func newGeneratorFixture(t *testing.T) *generatorFixture {
	f := &generatorFixture{}
	f.t = t
	f.l3portmanager = ostesting.NewMockL3PortManager()
	f.serviceLister = []*corev1.Service{}
	f.nodeLister = []*corev1.Node{}
	f.kubeobjects = []runtime.Object{}

	for i := 1; i <= 5; i++ {
		f.addNode(&corev1.Node{
			TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("kubernetes-node-%d", i),
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{Type: corev1.NodeHostName, Address: fmt.Sprintf("kubernetes-node-%d", i)},
					{Type: corev1.NodeInternalIP, Address: fmt.Sprintf("192.168.1.%d", i)},
					{Type: corev1.NodeExternalIP, Address: fmt.Sprintf("192.0.2.%d", i)},
				},
			},
		})
	}

	return f
}

func (f *generatorFixture) addNode(node *corev1.Node) {
	f.nodeLister = append(f.nodeLister, node)
	f.kubeobjects = append(f.kubeobjects, node)
}

func (f *generatorFixture) newGenerator() (*DefaultLoadBalancerModelGenerator, kubeinformers.SharedInformerFactory) {
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())
	services := k8sI.Core().V1().Services()
	nodes := k8sI.Core().V1().Nodes()

	for _, s := range f.serviceLister {
		services.Informer().GetIndexer().Add(s)
	}

	for _, n := range f.nodeLister {
		nodes.Informer().GetIndexer().Add(n)
	}

	g := NewDefaultLoadBalancerModelGenerator(
		f.l3portmanager,
		services.Lister(),
		nodes.Lister(),
	)
	return g, k8sI
}

func (f *generatorFixture) addService(svc *corev1.Service) {
	f.serviceLister = append(f.serviceLister, svc)
	f.kubeobjects = append(f.kubeobjects, svc)
}

func (f *generatorFixture) runWith(body func(g *DefaultLoadBalancerModelGenerator)) {
	g, k8sI := f.newGenerator()
	stopCh := make(chan struct{})
	defer close(stopCh)
	k8sI.Start(stopCh)

	body(g)

	f.l3portmanager.AssertExpectations(f.t)
}

func TestReturnsEmptyModelForEmptyAssignment(t *testing.T) {
	f := newGeneratorFixture(t)

	f.runWith(func(g *DefaultLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(map[string]string{})
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, len(m.Ingress))
	})
}

func TestReturnsEmptyModelForNilAssignment(t *testing.T) {
	f := newGeneratorFixture(t)

	f.runWith(func(g *DefaultLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(nil)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, len(m.Ingress))
	})
}

func TestSinglePortSingleServiceAssignment(t *testing.T) {
	f := newGeneratorFixture(t)

	svc := newService("svc-1")
	svc.Spec.Ports = []corev1.ServicePort{
		{Port: 80, NodePort: 31234, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc)

	a := map[string]string{
		model.FromService(svc).ToKey(): "port-id-1",
	}

	f.l3portmanager.On("GetExternalAddress", "port-id-1").Return("external-ip-1", "", nil).Times(1)

	f.runWith(func(g *DefaultLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 1, len(m.Ingress))

		i := m.Ingress[0]
		assert.Equal(t, "external-ip-1", i.Address)
		assert.Equal(t, 1, len(i.Ports))

		p := i.Ports[0]
		assert.Equal(t, corev1.ProtocolTCP, p.Protocol)
		assert.Equal(t, int32(80), p.InboundPort)
		assert.Equal(t, int32(31234), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}
	})
}

func TestSinglePortMultiServiceAssignment(t *testing.T) {
	f := newGeneratorFixture(t)

	svc1 := newService("svc-1")
	svc1.Spec.Ports = []corev1.ServicePort{
		{Port: 80, NodePort: 31234, Protocol: corev1.ProtocolTCP},
		{Port: 443, NodePort: 31235, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc1)

	svc2 := newService("svc-2")
	svc2.Spec.Ports = []corev1.ServicePort{
		{Port: 53, NodePort: 31236, Protocol: corev1.ProtocolUDP},
	}
	f.addService(svc2)

	a := map[string]string{
		model.FromService(svc1).ToKey(): "port-id-1",
		model.FromService(svc2).ToKey(): "port-id-1",
	}

	f.l3portmanager.On("GetExternalAddress", "port-id-1").Return("external-ip-1", "", nil).Times(1)

	f.runWith(func(g *DefaultLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 1, len(m.Ingress))

		i := m.Ingress[0]
		assert.Equal(t, "external-ip-1", i.Address)
		assert.Equal(t, 3, len(i.Ports))

		var p model.PortForward

		p = i.Ports[0]
		assert.Equal(t, corev1.ProtocolUDP, p.Protocol)
		assert.Equal(t, int32(53), p.InboundPort)
		assert.Equal(t, int32(31236), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}

		p = i.Ports[1]
		assert.Equal(t, corev1.ProtocolTCP, p.Protocol)
		assert.Equal(t, int32(80), p.InboundPort)
		assert.Equal(t, int32(31234), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}

		p = i.Ports[2]
		assert.Equal(t, corev1.ProtocolTCP, p.Protocol)
		assert.Equal(t, int32(443), p.InboundPort)
		assert.Equal(t, int32(31235), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}
	})
}

func TestMultiPortSingleServiceAssignment(t *testing.T) {
	f := newGeneratorFixture(t)

	svc1 := newService("svc-1")
	svc1.Spec.Ports = []corev1.ServicePort{
		{Port: 80, NodePort: 31234, Protocol: corev1.ProtocolTCP},
		{Port: 443, NodePort: 31235, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc1)

	svc2 := newService("svc-2")
	svc2.Spec.Ports = []corev1.ServicePort{
		{Port: 53, NodePort: 31236, Protocol: corev1.ProtocolUDP},
	}
	f.addService(svc2)

	a := map[string]string{
		model.FromService(svc1).ToKey(): "port-id-1",
		model.FromService(svc2).ToKey(): "port-id-2",
	}

	f.l3portmanager.On("GetExternalAddress", "port-id-1").Return("external-ip-1", "", nil).Times(1)
	f.l3portmanager.On("GetExternalAddress", "port-id-2").Return("external-ip-2", "", nil).Times(1)

	f.runWith(func(g *DefaultLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 2, len(m.Ingress))

		var i model.IngressIP

		i = m.Ingress[0]
		assert.Equal(t, "external-ip-1", i.Address)
		assert.Equal(t, 2, len(i.Ports))

		var p model.PortForward

		p = i.Ports[0]
		assert.Equal(t, corev1.ProtocolTCP, p.Protocol)
		assert.Equal(t, int32(80), p.InboundPort)
		assert.Equal(t, int32(31234), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}

		p = i.Ports[1]
		assert.Equal(t, corev1.ProtocolTCP, p.Protocol)
		assert.Equal(t, int32(443), p.InboundPort)
		assert.Equal(t, int32(31235), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}

		i = m.Ingress[1]
		assert.Equal(t, "external-ip-2", i.Address)
		assert.Equal(t, 1, len(i.Ports))

		p = i.Ports[0]
		assert.Equal(t, corev1.ProtocolUDP, p.Protocol)
		assert.Equal(t, int32(53), p.InboundPort)
		assert.Equal(t, int32(31236), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}
	})
}

func TestMultiPortMultiServiceAssignment(t *testing.T) {
	f := newGeneratorFixture(t)

	svc1 := newService("svc-1")
	svc1.Spec.Ports = []corev1.ServicePort{
		{Port: 80, NodePort: 31234, Protocol: corev1.ProtocolTCP},
		{Port: 443, NodePort: 31235, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc1)

	svc2 := newService("svc-2")
	svc2.Spec.Ports = []corev1.ServicePort{
		{Port: 53, NodePort: 31236, Protocol: corev1.ProtocolUDP},
	}
	f.addService(svc2)

	svc3 := newService("svc-3")
	svc3.Spec.Ports = []corev1.ServicePort{
		{Port: 9090, NodePort: 31237, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc3)

	a := map[string]string{
		model.FromService(svc1).ToKey(): "port-id-1",
		model.FromService(svc2).ToKey(): "port-id-2",
		model.FromService(svc3).ToKey(): "port-id-2",
	}

	f.l3portmanager.On("GetExternalAddress", "port-id-1").Return("external-ip-1", "", nil).Times(1)
	f.l3portmanager.On("GetExternalAddress", "port-id-2").Return("external-ip-2", "", nil).Times(1)

	f.runWith(func(g *DefaultLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 2, len(m.Ingress))

		var i model.IngressIP

		i = m.Ingress[0]
		assert.Equal(t, "external-ip-1", i.Address)
		assert.Equal(t, 2, len(i.Ports))

		var p model.PortForward

		p = i.Ports[0]
		assert.Equal(t, corev1.ProtocolTCP, p.Protocol)
		assert.Equal(t, int32(80), p.InboundPort)
		assert.Equal(t, int32(31234), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}

		p = i.Ports[1]
		assert.Equal(t, corev1.ProtocolTCP, p.Protocol)
		assert.Equal(t, int32(443), p.InboundPort)
		assert.Equal(t, int32(31235), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}

		i = m.Ingress[1]
		assert.Equal(t, "external-ip-2", i.Address)
		assert.Equal(t, 2, len(i.Ports))

		p = i.Ports[0]
		assert.Equal(t, corev1.ProtocolUDP, p.Protocol)
		assert.Equal(t, int32(53), p.InboundPort)
		assert.Equal(t, int32(31236), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}

		p = i.Ports[1]
		assert.Equal(t, corev1.ProtocolTCP, p.Protocol)
		assert.Equal(t, int32(9090), p.InboundPort)
		assert.Equal(t, int32(31237), p.DestinationPort)

		assert.Equal(t, len(f.nodeLister), len(p.DestinationAddresses))
		for _, node := range f.nodeLister {
			assert.Contains(t, p.DestinationAddresses, node.Status.Addresses[1].Address)
		}
	})
}
