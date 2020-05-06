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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	ostesting "github.com/cloudandheat/ch-k8s-lbaas/internal/openstack/testing"
)

type clusterIPGeneratorFixture struct {
	t *testing.T

	l3portmanager *ostesting.MockL3PortManager

	kubeclient    *k8sfake.Clientset
	serviceLister []*corev1.Service
	kubeobjects   []runtime.Object
}

func newClusterIPGeneratorFixture(t *testing.T) *clusterIPGeneratorFixture {
	f := &clusterIPGeneratorFixture{}
	f.t = t
	f.l3portmanager = ostesting.NewMockL3PortManager()
	f.serviceLister = []*corev1.Service{}
	f.kubeobjects = []runtime.Object{}

	return f
}

func (f *clusterIPGeneratorFixture) newGenerator() (*ClusterIPLoadBalancerModelGenerator, kubeinformers.SharedInformerFactory) {
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())
	services := k8sI.Core().V1().Services()

	for _, s := range f.serviceLister {
		services.Informer().GetIndexer().Add(s)
	}

	g := NewClusterIPLoadBalancerModelGenerator(
		f.l3portmanager,
		services.Lister(),
	)
	return g, k8sI
}

func (f *clusterIPGeneratorFixture) addService(svc *corev1.Service) {
	f.serviceLister = append(f.serviceLister, svc)
	f.kubeobjects = append(f.kubeobjects, svc)
}

func (f *clusterIPGeneratorFixture) runWith(body func(g *ClusterIPLoadBalancerModelGenerator)) {
	g, k8sI := f.newGenerator()
	stopCh := make(chan struct{})
	defer close(stopCh)
	k8sI.Start(stopCh)

	body(g)

	f.l3portmanager.AssertExpectations(f.t)
}

func TestClusterIPReturnsEmptyModelForEmptyAssignment(t *testing.T) {
	f := newClusterIPGeneratorFixture(t)

	f.runWith(func(g *ClusterIPLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(map[string]string{})
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, len(m.Ingress))
	})
}

func TestClusterIPReturnsEmptyModelForNilAssignment(t *testing.T) {
	f := newClusterIPGeneratorFixture(t)

	f.runWith(func(g *ClusterIPLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(nil)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, len(m.Ingress))
	})
}

func TestClusterIPSinglePortSingleServiceAssignment(t *testing.T) {
	f := newClusterIPGeneratorFixture(t)

	svc := newService("svc-1")
	svc.Spec.ClusterIP = "10.20.30.40"
	svc.Spec.Ports = []corev1.ServicePort{
		{Port: 80, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc)

	a := map[string]string{
		model.FromService(svc).ToKey(): "port-id-1",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("ingress-ip-1", nil).Times(1)

	f.runWith(func(g *ClusterIPLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 1, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "ingress-ip-1", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 1, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(80), p.DestinationPort)

				assert.Equal(t, []string{"10.20.30.40"}, p.DestinationAddresses)
			})
		})
	})
}

func TestClusterIPMultiPortSingleServiceAssignment(t *testing.T) {
	f := newClusterIPGeneratorFixture(t)

	svc1 := newService("svc-1")
	svc1.Spec.ClusterIP = "10.0.0.1"
	svc1.Spec.Ports = []corev1.ServicePort{
		{Port: 80, Protocol: corev1.ProtocolTCP},
		{Port: 443, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc1)

	svc2 := newService("svc-2")
	svc2.Spec.ClusterIP = "10.0.0.2"
	svc2.Spec.Ports = []corev1.ServicePort{
		{Port: 53, Protocol: corev1.ProtocolUDP},
	}
	f.addService(svc2)

	a := map[string]string{
		model.FromService(svc1).ToKey(): "port-id-1",
		model.FromService(svc2).ToKey(): "port-id-2",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("ingress-ip-1", nil).Times(1)
	f.l3portmanager.On("GetInternalAddress", "port-id-2").Return("ingress-ip-2", nil).Times(1)

	f.runWith(func(g *ClusterIPLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 2, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "ingress-ip-1", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 2, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(80), p.DestinationPort)
				assert.Equal(t, []string{"10.0.0.1"}, p.DestinationAddresses)
			})

			anyPort(t, i.Ports, 443, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(443), p.DestinationPort)
				assert.Equal(t, []string{"10.0.0.1"}, p.DestinationAddresses)
			})
		})

		anyIngressIP(t, m.Ingress, "ingress-ip-2", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 1, len(i.Ports))

			anyPort(t, i.Ports, 53, corev1.ProtocolUDP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(53), p.DestinationPort)
				assert.Equal(t, []string{"10.0.0.2"}, p.DestinationAddresses)
			})
		})
	})
}

func TestClusterIPMultiPortMultiServiceAssignment(t *testing.T) {
	f := newClusterIPGeneratorFixture(t)

	svc1 := newService("svc-1")
	svc1.Spec.ClusterIP = "10.0.0.1"
	svc1.Spec.Ports = []corev1.ServicePort{
		{Port: 80, Protocol: corev1.ProtocolTCP},
		{Port: 443, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc1)

	svc2 := newService("svc-2")
	svc2.Spec.ClusterIP = "10.0.0.2"
	svc2.Spec.Ports = []corev1.ServicePort{
		{Port: 53, Protocol: corev1.ProtocolUDP},
		{Port: 53, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc2)

	svc3 := newService("svc-3")
	svc3.Spec.ClusterIP = "10.0.0.3"
	svc3.Spec.Ports = []corev1.ServicePort{
		{Port: 9090, Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc3)

	a := map[string]string{
		model.FromService(svc1).ToKey(): "port-id-1",
		model.FromService(svc2).ToKey(): "port-id-2",
		model.FromService(svc3).ToKey(): "port-id-2",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("ingress-ip-1", nil).Times(1)
	f.l3portmanager.On("GetInternalAddress", "port-id-2").Return("ingress-ip-2", nil).Times(1)

	f.runWith(func(g *ClusterIPLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 2, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "ingress-ip-1", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 2, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(80), p.DestinationPort)
				assert.Equal(t, []string{"10.0.0.1"}, p.DestinationAddresses)
			})

			anyPort(t, i.Ports, 443, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(443), p.DestinationPort)
				assert.Equal(t, []string{"10.0.0.1"}, p.DestinationAddresses)
			})
		})

		anyIngressIP(t, m.Ingress, "ingress-ip-2", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 3, len(i.Ports))

			anyPort(t, i.Ports, 53, corev1.ProtocolUDP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(53), p.DestinationPort)
				assert.Equal(t, []string{"10.0.0.2"}, p.DestinationAddresses)
			})

			anyPort(t, i.Ports, 53, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(53), p.DestinationPort)
				assert.Equal(t, []string{"10.0.0.2"}, p.DestinationAddresses)
			})

			anyPort(t, i.Ports, 9090, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(9090), p.DestinationPort)
				assert.Equal(t, []string{"10.0.0.3"}, p.DestinationAddresses)
			})
		})
	})
}
