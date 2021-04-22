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
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	ostesting "github.com/cloudandheat/ch-k8s-lbaas/internal/openstack/testing"
)

type nodePortGeneratorFixture struct {
	t *testing.T

	l3portmanager *ostesting.MockL3PortManager

	kubeclient    *k8sfake.Clientset
	serviceLister []*corev1.Service
	nodeLister    []*corev1.Node
	kubeobjects   []runtime.Object
}

func newNodePortGeneratorFixture(t *testing.T) *nodePortGeneratorFixture {
	f := &nodePortGeneratorFixture{}
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

func (f *nodePortGeneratorFixture) addNode(node *corev1.Node) {
	f.nodeLister = append(f.nodeLister, node)
	f.kubeobjects = append(f.kubeobjects, node)
}

func (f *nodePortGeneratorFixture) newGenerator() (*NodePortLoadBalancerModelGenerator, kubeinformers.SharedInformerFactory) {
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

	g := NewNodePortLoadBalancerModelGenerator(
		f.l3portmanager,
		services.Lister(),
		nodes.Lister(),
	)
	return g, k8sI
}

func (f *nodePortGeneratorFixture) addService(svc *corev1.Service) {
	f.serviceLister = append(f.serviceLister, svc)
	f.kubeobjects = append(f.kubeobjects, svc)
}

func (f *nodePortGeneratorFixture) runWith(body func(g *NodePortLoadBalancerModelGenerator)) {
	g, k8sI := f.newGenerator()
	stopCh := make(chan struct{})
	defer close(stopCh)
	k8sI.Start(stopCh)

	body(g)

	f.l3portmanager.AssertExpectations(f.t)
}

func anyIngressIP(t *testing.T, items []model.IngressIP, address string, testfunc func(t *testing.T, i model.IngressIP)) {
	assert.Conditionf(t, func() bool {
		for _, item := range items {
			if item.Address != address {
				continue
			}
			testfunc(t, item)
			return true
		}
		return false
	}, "no Ingress found for address %s in %#v", address, items)
}

func anyPort(t *testing.T, items []model.PortForward, inboundPort int32, protocol corev1.Protocol, testfunc func(t *testing.T, p model.PortForward)) {
	assert.Conditionf(t, func() bool {
		for _, item := range items {
			if item.InboundPort != inboundPort || item.Protocol != protocol {
				continue
			}
			testfunc(t, item)
			return true
		}
		return false
	}, "no PortForward found for %d/%s in %#v", inboundPort, protocol, items)
}

func (f *nodePortGeneratorFixture) matchDestinationAddresses(p model.PortForward) {
	assert.Equal(f.t, len(f.nodeLister), len(p.DestinationAddresses))
	for _, node := range f.nodeLister {
		assert.Contains(f.t, p.DestinationAddresses, node.Status.Addresses[1].Address)
	}
}

func TestNodePortReturnsEmptyModelForEmptyAssignment(t *testing.T) {
	f := newNodePortGeneratorFixture(t)

	f.runWith(func(g *NodePortLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(map[string]string{})
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, len(m.Ingress))
	})
}

func TestNodePortReturnsEmptyModelForNilAssignment(t *testing.T) {
	f := newNodePortGeneratorFixture(t)

	f.runWith(func(g *NodePortLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(nil)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, len(m.Ingress))
	})
}

func TestNodePortSinglePortSingleServiceAssignment(t *testing.T) {
	f := newNodePortGeneratorFixture(t)

	svc := newService("svc-1")
	svc.Spec.Ports = []corev1.ServicePort{
		{Port: 80, NodePort: 31234, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc)

	a := map[string]string{
		model.FromService(svc).ToKey(): "port-id-1",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("10.0.0.2", nil).Times(1)

	f.runWith(func(g *NodePortLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 1, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "10.0.0.2", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 1, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31234), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})
		})
	})
}

func TestNodePortSinglePortMultiServiceAssignment(t *testing.T) {
	f := newNodePortGeneratorFixture(t)

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

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("10.0.0.2", nil).Times(1)

	f.runWith(func(g *NodePortLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 1, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "10.0.0.2", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 3, len(i.Ports))

			anyPort(t, i.Ports, 53, corev1.ProtocolUDP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31236), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31234), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})

			anyPort(t, i.Ports, 443, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31235), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})
		})
	})
}

func TestNodePortMultiPortSingleServiceAssignment(t *testing.T) {
	f := newNodePortGeneratorFixture(t)

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

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("10.0.0.2", nil).Times(1)
	f.l3portmanager.On("GetInternalAddress", "port-id-2").Return("10.0.0.3", nil).Times(1)

	f.runWith(func(g *NodePortLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 2, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "10.0.0.2", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 2, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31234), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})

			anyPort(t, i.Ports, 443, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31235), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})
		})

		anyIngressIP(t, m.Ingress, "10.0.0.3", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 1, len(i.Ports))

			anyPort(t, i.Ports, 53, corev1.ProtocolUDP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31236), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})
		})
	})
}

func TestNodePortMultiPortMultiServiceAssignment(t *testing.T) {
	f := newNodePortGeneratorFixture(t)

	svc1 := newService("svc-1")
	svc1.Spec.Ports = []corev1.ServicePort{
		{Port: 80, NodePort: 31234, Protocol: corev1.ProtocolTCP},
		{Port: 443, NodePort: 31235, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc1)

	svc2 := newService("svc-2")
	svc2.Spec.Ports = []corev1.ServicePort{
		{Port: 53, NodePort: 31236, Protocol: corev1.ProtocolUDP},
		{Port: 53, NodePort: 31236, Protocol: corev1.ProtocolTCP},
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

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("10.0.0.2", nil).Times(1)
	f.l3portmanager.On("GetInternalAddress", "port-id-2").Return("10.0.0.3", nil).Times(1)

	f.runWith(func(g *NodePortLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 2, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "10.0.0.2", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 2, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31234), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})

			anyPort(t, i.Ports, 443, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31235), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})
		})

		anyIngressIP(t, m.Ingress, "10.0.0.3", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 3, len(i.Ports))

			anyPort(t, i.Ports, 53, corev1.ProtocolUDP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31236), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})

			anyPort(t, i.Ports, 53, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31236), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})

			anyPort(t, i.Ports, 9090, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(31237), p.DestinationPort)

				f.matchDestinationAddresses(p)
			})
		})
	})
}
