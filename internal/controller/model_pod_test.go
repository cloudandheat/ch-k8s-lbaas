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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	ostesting "github.com/cloudandheat/ch-k8s-lbaas/internal/openstack/testing"
)

type podGeneratorFixture struct {
	t *testing.T

	l3portmanager *ostesting.MockL3PortManager

	kubeclient      *k8sfake.Clientset
	serviceLister   []*corev1.Service
	endpointsLister []*corev1.Endpoints
	kubeobjects     []runtime.Object
}

func newPodGeneratorFixture(t *testing.T) *podGeneratorFixture {
	f := &podGeneratorFixture{}
	f.t = t
	f.l3portmanager = ostesting.NewMockL3PortManager()
	f.serviceLister = []*corev1.Service{}
	f.endpointsLister = []*corev1.Endpoints{}
	f.kubeobjects = []runtime.Object{}

	return f
}

func (f *podGeneratorFixture) newGenerator() (*PodLoadBalancerModelGenerator, kubeinformers.SharedInformerFactory) {
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())
	services := k8sI.Core().V1().Services()
	endpoints := k8sI.Core().V1().Endpoints()

	for _, s := range f.serviceLister {
		services.Informer().GetIndexer().Add(s)
	}

	for _, e := range f.endpointsLister {
		endpoints.Informer().GetIndexer().Add(e)
	}

	g := NewPodLoadBalancerModelGenerator(
		f.l3portmanager,
		services.Lister(),
		endpoints.Lister(),
	)
	return g, k8sI
}

func (f *podGeneratorFixture) addService(svc *corev1.Service) {
	f.serviceLister = append(f.serviceLister, svc)
	f.kubeobjects = append(f.kubeobjects, svc)
}

func (f *podGeneratorFixture) addEndpoints(svc *corev1.Endpoints) {
	f.endpointsLister = append(f.endpointsLister, svc)
	f.kubeobjects = append(f.kubeobjects, svc)
}

func (f *podGeneratorFixture) runWith(body func(g *PodLoadBalancerModelGenerator)) {
	g, k8sI := f.newGenerator()
	stopCh := make(chan struct{})
	defer close(stopCh)
	k8sI.Start(stopCh)

	body(g)

	f.l3portmanager.AssertExpectations(f.t)
}

func newEndpoints(name string) *corev1.Endpoints {
	return &corev1.Endpoints{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
	}
}

func TestPodReturnsEmptyModelForEmptyAssignment(t *testing.T) {
	f := newPodGeneratorFixture(t)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(map[string]string{})
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, len(m.Ingress))
	})
}

func TestPodReturnsEmptyModelForNilAssignment(t *testing.T) {
	f := newPodGeneratorFixture(t)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(nil)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, len(m.Ingress))
	})
}

func TestPodSinglePortSingleServiceAssignment(t *testing.T) {
	f := newPodGeneratorFixture(t)

	ep1 := newEndpoints("svc-1")
	ep1.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.0.1"},
				{IP: "10.224.1.1"},
				{IP: "10.224.2.1"},
			},
			Ports: []corev1.EndpointPort{
				{Port: 8080, Protocol: corev1.ProtocolTCP},
				{Port: 8443, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	f.addEndpoints(ep1)

	svc := newService("svc-1")
	svc.Spec.Ports = []corev1.ServicePort{
		{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(8080)},
	}
	f.addService(svc)

	a := map[string]string{
		model.FromService(svc).ToKey(): "port-id-1",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("ingress-ip-1", nil).Times(1)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 1, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "ingress-ip-1", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 1, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(8080), p.DestinationPort)

				assert.Equal(t, []string{"10.224.0.1", "10.224.1.1", "10.224.2.1"}, p.DestinationAddresses)
			})
		})
	})
}

func TestPodSinglePortSingleServiceAssignmentByName(t *testing.T) {
	f := newPodGeneratorFixture(t)

	ep1 := newEndpoints("svc-1")
	ep1.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.0.1"},
				{IP: "10.224.1.1"},
				{IP: "10.224.2.1"},
			},
			Ports: []corev1.EndpointPort{
				{Port: 8080, Protocol: corev1.ProtocolTCP},
				{Port: 8443, Name: "http", Protocol: corev1.ProtocolTCP},
			},
		},
	}
	f.addEndpoints(ep1)

	svc := newService("svc-1")
	svc.Spec.Ports = []corev1.ServicePort{
		{Port: 80, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc)

	a := map[string]string{
		model.FromService(svc).ToKey(): "port-id-1",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("ingress-ip-1", nil).Times(1)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 1, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "ingress-ip-1", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 1, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(8443), p.DestinationPort)

				assert.Equal(t, []string{"10.224.0.1", "10.224.1.1", "10.224.2.1"}, p.DestinationAddresses)
			})
		})
	})
}

func TestPodSinglePortMultiSubsetSingleServiceAssignment(t *testing.T) {
	f := newPodGeneratorFixture(t)

	ep1 := newEndpoints("svc-1")
	ep1.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.0.1"},
				{IP: "10.224.2.1"},
			},
			Ports: []corev1.EndpointPort{
				{Port: 8080, Protocol: corev1.ProtocolTCP},
				{Port: 8443, Protocol: corev1.ProtocolTCP},
			},
		},
		// NOTE: this test shows that we currently do not support endpoint
		// objects with more than one subset, since the ports may differ.
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.1.1"},
			},
			Ports: []corev1.EndpointPort{
				{Port: 8081, Protocol: corev1.ProtocolTCP},
				{Port: 8444, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	f.addEndpoints(ep1)

	svc := newService("svc-1")
	svc.Spec.Ports = []corev1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc)

	a := map[string]string{
		model.FromService(svc).ToKey(): "port-id-1",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("ingress-ip-1", nil).Times(1)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 1, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "ingress-ip-1", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 1, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(8080), p.DestinationPort)

				assert.Equal(t, []string{"10.224.0.1", "10.224.2.1"}, p.DestinationAddresses)
			})
		})
	})
}

func TestPodMultiPortSingleServiceAssignment(t *testing.T) {
	f := newPodGeneratorFixture(t)

	ep1 := newEndpoints("svc-1")
	ep1.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.0.1"},
				{IP: "10.224.1.1"},
				{IP: "10.224.2.1"},
			},
			Ports: []corev1.EndpointPort{
				{Port: 8080, Protocol: corev1.ProtocolTCP},
				{Port: 8443, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	f.addEndpoints(ep1)

	svc1 := newService("svc-1")
	svc1.Spec.Ports = []corev1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
		{Port: 443, TargetPort: intstr.FromInt(8443), Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc1)

	ep2 := newEndpoints("svc-2")
	ep2.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.0.2"},
				{IP: "10.224.1.2"},
			},
			Ports: []corev1.EndpointPort{{Port: 53, Protocol: corev1.ProtocolUDP}},
		},
	}
	f.addEndpoints(ep2)

	svc2 := newService("svc-2")
	svc2.Spec.Ports = []corev1.ServicePort{
		{Port: 53, TargetPort: intstr.FromInt(53), Protocol: corev1.ProtocolUDP},
	}
	f.addService(svc2)

	a := map[string]string{
		model.FromService(svc1).ToKey(): "port-id-1",
		model.FromService(svc2).ToKey(): "port-id-2",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("ingress-ip-1", nil).Times(1)
	f.l3portmanager.On("GetInternalAddress", "port-id-2").Return("ingress-ip-2", nil).Times(1)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 2, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "ingress-ip-1", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 2, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(8080), p.DestinationPort)
				assert.Equal(t, []string{"10.224.0.1", "10.224.1.1", "10.224.2.1"}, p.DestinationAddresses)
			})

			anyPort(t, i.Ports, 443, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(8443), p.DestinationPort)
				assert.Equal(t, []string{"10.224.0.1", "10.224.1.1", "10.224.2.1"}, p.DestinationAddresses)
			})
		})

		anyIngressIP(t, m.Ingress, "ingress-ip-2", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 1, len(i.Ports))

			anyPort(t, i.Ports, 53, corev1.ProtocolUDP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(53), p.DestinationPort)
				assert.Equal(t, []string{"10.224.0.2", "10.224.1.2"}, p.DestinationAddresses)
			})
		})
	})
}

func TestPodMultiPortMultiServiceAssignment(t *testing.T) {
	f := newPodGeneratorFixture(t)

	ep1 := newEndpoints("svc-1")
	ep1.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.0.1"},
				{IP: "10.224.1.1"},
				{IP: "10.224.2.1"},
			},
			Ports: []corev1.EndpointPort{
				{Port: 8080, Protocol: corev1.ProtocolTCP},
				{Port: 8443, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	f.addEndpoints(ep1)

	svc1 := newService("svc-1")
	svc1.Spec.Ports = []corev1.ServicePort{
		{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(8080)},
		{Port: 443, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(8443)},
	}
	f.addService(svc1)

	ep2 := newEndpoints("svc-2")
	ep2.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.0.2"},
				{IP: "10.224.1.2"},
			},
			Ports: []corev1.EndpointPort{
				{Port: 53, Name: "dns", Protocol: corev1.ProtocolUDP},
				{Port: 5353, Name: "dns", Protocol: corev1.ProtocolTCP},
			},
		},
	}
	f.addEndpoints(ep2)

	svc2 := newService("svc-2")
	svc2.Spec.Ports = []corev1.ServicePort{
		{Port: 53, TargetPort: intstr.FromString("dns"), Protocol: corev1.ProtocolUDP},
		{Port: 53, TargetPort: intstr.FromString("dns"), Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc2)

	ep3 := newEndpoints("svc-3")
	ep3.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.224.0.3"},
				{IP: "10.224.1.3"},
			},
			Ports: []corev1.EndpointPort{
				{Port: 9090, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	f.addEndpoints(ep3)

	svc3 := newService("svc-3")
	svc3.Spec.Ports = []corev1.ServicePort{
		{Port: 9090, TargetPort: intstr.FromInt(9090), Protocol: corev1.ProtocolTCP},
	}
	f.addService(svc3)

	a := map[string]string{
		model.FromService(svc1).ToKey(): "port-id-1",
		model.FromService(svc2).ToKey(): "port-id-2",
		model.FromService(svc3).ToKey(): "port-id-2",
	}

	f.l3portmanager.On("GetInternalAddress", "port-id-1").Return("ingress-ip-1", nil).Times(1)
	f.l3portmanager.On("GetInternalAddress", "port-id-2").Return("ingress-ip-2", nil).Times(1)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(a)
		assert.Nil(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 2, len(m.Ingress))

		anyIngressIP(t, m.Ingress, "ingress-ip-1", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 2, len(i.Ports))

			anyPort(t, i.Ports, 80, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(8080), p.DestinationPort)
				assert.Equal(t, []string{"10.224.0.1", "10.224.1.1", "10.224.2.1"}, p.DestinationAddresses)
			})

			anyPort(t, i.Ports, 443, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(8443), p.DestinationPort)
				assert.Equal(t, []string{"10.224.0.1", "10.224.1.1", "10.224.2.1"}, p.DestinationAddresses)
			})
		})

		anyIngressIP(t, m.Ingress, "ingress-ip-2", func(t *testing.T, i model.IngressIP) {
			assert.Equal(t, 3, len(i.Ports))

			anyPort(t, i.Ports, 53, corev1.ProtocolUDP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(53), p.DestinationPort)
				assert.Equal(t, []string{"10.224.0.2", "10.224.1.2"}, p.DestinationAddresses)
			})

			anyPort(t, i.Ports, 53, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(5353), p.DestinationPort)
				assert.Equal(t, []string{"10.224.0.2", "10.224.1.2"}, p.DestinationAddresses)
			})

			anyPort(t, i.Ports, 9090, corev1.ProtocolTCP, func(t *testing.T, p model.PortForward) {
				assert.Equal(t, int32(9090), p.DestinationPort)
				assert.Equal(t, []string{"10.224.0.3", "10.224.1.3"}, p.DestinationAddresses)
			})
		})
	})
}
