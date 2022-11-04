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
	networkingv1 "k8s.io/api/networking/v1"
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

	kubeclient          *k8sfake.Clientset
	serviceLister       []*corev1.Service
	endpointsLister     []*corev1.Endpoints
	networkpolicyLister []*networkingv1.NetworkPolicy
	podLister           []*corev1.Pod
	kubeobjects         []runtime.Object
}

func newPodGeneratorFixture(t *testing.T) *podGeneratorFixture {
	f := &podGeneratorFixture{}
	f.t = t
	f.l3portmanager = ostesting.NewMockL3PortManager()
	f.serviceLister = []*corev1.Service{}
	f.endpointsLister = []*corev1.Endpoints{}
	f.networkpolicyLister = []*networkingv1.NetworkPolicy{}
	f.podLister = []*corev1.Pod{}
	f.kubeobjects = []runtime.Object{}

	return f
}

func (f *podGeneratorFixture) newGenerator() (*PodLoadBalancerModelGenerator, kubeinformers.SharedInformerFactory) {
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())
	services := k8sI.Core().V1().Services()
	endpoints := k8sI.Core().V1().Endpoints()
	networkpolicies := k8sI.Networking().V1().NetworkPolicies()
	pods := k8sI.Core().V1().Pods()

	for _, s := range f.serviceLister {
		services.Informer().GetIndexer().Add(s)
	}

	for _, e := range f.endpointsLister {
		endpoints.Informer().GetIndexer().Add(e)
	}

	for _, pol := range f.networkpolicyLister {
		networkpolicies.Informer().GetIndexer().Add(pol)
	}

	for _, pod := range f.podLister {
		pods.Informer().GetIndexer().Add(pod)
	}

	g := NewPodLoadBalancerModelGenerator(
		f.l3portmanager,
		services.Lister(),
		endpoints.Lister(),
		networkpolicies.Lister(),
		pods.Lister(),
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

func (f *podGeneratorFixture) addNetworkPolicy(pol *networkingv1.NetworkPolicy) {
	f.networkpolicyLister = append(f.networkpolicyLister, pol)
	f.kubeobjects = append(f.kubeobjects, pol)
}

func (f *podGeneratorFixture) addPod(pod *corev1.Pod) {
	f.podLister = append(f.podLister, pod)
	f.kubeobjects = append(f.kubeobjects, pod)
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

func newPod(name string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
	}
}

func newNetworkPolicy(name string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{APIVersion: networkingv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
	}
}

func newNetworkPolicyPort(protocol corev1.Protocol, port int32, endPort int32) networkingv1.NetworkPolicyPort {
	newPort := intstr.FromInt(int(port))
	var newEndPort *int32
	if endPort != 0 {
		newEndPort = &endPort
	}
	return networkingv1.NetworkPolicyPort{
		Protocol: &protocol,
		Port:     &newPort,
		EndPort:  newEndPort,
	}
}

func stringInList(items []string, s string) bool {
	for _, element := range items {
		if element == s {
			return true
		}
	}
	return false
}

// Looks for the policy assignment matching `address` in `items` and applies the test function `testfunc` to it
func anyPolicyAssignment(t *testing.T, items []model.PolicyAssignment, address string, testfunc func(t *testing.T, p model.PolicyAssignment)) {
	assert.Conditionf(t, func() bool {
		for _, item := range items {
			if item.Address != address {
				continue
			}
			testfunc(t, item)
			return true
		}
		return false
	}, "no PolicyAssignment found for %s in %#v", address, items)
}

// Looks for the network policy matching `name` in `items` and applies the test function `testfunc` to it
func anyNetworkPolicy(t *testing.T, items []model.NetworkPolicy, name string, testfunc func(t *testing.T, p model.NetworkPolicy)) {
	assert.Condition(t, func() bool {
		for _, item := range items {
			if item.Name != name {
				continue
			}
			testfunc(t, item)
			return true
		}
		return false
	}, "no NetworkPolicy found for %s in %#v", name, items)
}

// Looks for the network policy matching `name` in `items` and applies the test function `testfunc` to it
func anyAllowed(t *testing.T, items []model.NetworkPolicy, name string, testfunc func(t *testing.T, p model.NetworkPolicy)) {
	assert.Condition(t, func() bool {
		for _, item := range items {
			if item.Name != name {
				continue
			}
			testfunc(t, item)
			return true
		}
		return false
	}, "no NetworkPolicy found for %s in %#v", name, items)
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

func TestNetworkPolicyAssignments(t *testing.T) {
	f := newPodGeneratorFixture(t)

	pod1 := newPod("pod-1")
	pod1.ObjectMeta.Labels = map[string]string{}
	pod1.ObjectMeta.Labels["app.kubernetes.io/name"] = "app1"
	pod1.ObjectMeta.Labels["other"] = "1"
	pod1.Status.PodIPs = []corev1.PodIP{{IP: "10.224.1.1"}}
	f.addPod(pod1)

	pod2 := newPod("pod-2")
	pod2.ObjectMeta.Labels = map[string]string{}
	pod2.ObjectMeta.Labels["app.kubernetes.io/name"] = "app1"
	pod2.Status.PodIPs = []corev1.PodIP{{IP: "10.224.1.2"}}
	f.addPod(pod2)

	pod3 := newPod("pod-3")
	pod3.ObjectMeta.Labels = map[string]string{}
	pod3.ObjectMeta.Labels["other"] = "1"
	pod3.Status.PodIPs = []corev1.PodIP{{IP: "10.224.1.3"}}
	f.addPod(pod3)

	np1 := newNetworkPolicy("policy-1")
	np1.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np1.Spec.PodSelector.MatchLabels = map[string]string{"app.kubernetes.io/name": "app1"}
	np1.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{}}
	f.addNetworkPolicy(np1)

	np2 := newNetworkPolicy("policy-2")
	np2.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np2.Spec.PodSelector.MatchLabels = map[string]string{"other": "1"}
	np2.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{}}
	f.addNetworkPolicy(np2)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(nil)

		assert.Nil(t, err)
		assert.NotNil(t, m)

		assert.Equal(t, 3, len(m.PolicyAssignments))

		anyPolicyAssignment(t, m.PolicyAssignments, "10.224.1.1", func(t *testing.T, p model.PolicyAssignment) {
			assert.Equal(t, 2, len(p.NetworkPolicies))
			assert.True(t, stringInList(p.NetworkPolicies, "policy-1"))
			assert.True(t, stringInList(p.NetworkPolicies, "policy-2"))
		})

		anyPolicyAssignment(t, m.PolicyAssignments, "10.224.1.2", func(t *testing.T, p model.PolicyAssignment) {
			assert.Equal(t, 1, len(p.NetworkPolicies))
			assert.True(t, stringInList(p.NetworkPolicies, "policy-1"))
		})

		anyPolicyAssignment(t, m.PolicyAssignments, "10.224.1.3", func(t *testing.T, p model.PolicyAssignment) {
			assert.Equal(t, 1, len(p.NetworkPolicies))
			assert.True(t, stringInList(p.NetworkPolicies, "policy-2"))
		})
	})
}

func TestNetworkPolicies(t *testing.T) {
	f := newPodGeneratorFixture(t)

	np1 := newNetworkPolicy("allow-http")
	np1.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np1.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
		{
			Ports: []networkingv1.NetworkPolicyPort{
				newNetworkPolicyPort(corev1.ProtocolTCP, 80, 0),
			},
		},
	}
	f.addNetworkPolicy(np1)

	np2 := newNetworkPolicy("block-range")
	np2.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np2.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
		{
			From: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: "0.0.0.0/0",
						Except: []string{
							"192.168.2.0/24",
							"192.168.178.0/24",
						},
					},
				},
			},
		},
	}
	f.addNetworkPolicy(np2)

	np3 := newNetworkPolicy("allow-everything")
	np3.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np3.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
		{},
	}
	f.addNetworkPolicy(np3)

	np4 := newNetworkPolicy("block-everything")
	np4.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np4.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{}
	f.addNetworkPolicy(np4)

	np5 := newNetworkPolicy("multiple-ports-with-ranges")
	np5.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np5.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
		{
			Ports: []networkingv1.NetworkPolicyPort{
				newNetworkPolicyPort(corev1.ProtocolTCP, 2000, 3000),
				newNetworkPolicyPort(corev1.ProtocolUDP, 4000, 5000),
				newNetworkPolicyPort(corev1.ProtocolUDP, 6000, 7000),
			},
		},
	}
	f.addNetworkPolicy(np5)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(nil)

		assert.Nil(t, err)
		assert.NotNil(t, m)
		anyNetworkPolicy(t, m.NetworkPolicies, "allow-http", func(t *testing.T, p model.NetworkPolicy) {
			assert.Equal(t, 1, len(p.AllowedIngresses))
			assert.Equal(t, 0, len(p.AllowedIngresses[0].IPBlockFilters))
			assert.Equal(t, 1, len(p.AllowedIngresses[0].PortFilters))
			assert.Equal(t, corev1.ProtocolTCP, p.AllowedIngresses[0].PortFilters[0].Protocol)
			assert.Equal(t, int32(80), *p.AllowedIngresses[0].PortFilters[0].Port)
			assert.Nil(t, p.AllowedIngresses[0].PortFilters[0].EndPort)
		})

		anyNetworkPolicy(t, m.NetworkPolicies, "block-range", func(t *testing.T, p model.NetworkPolicy) {
			assert.Equal(t, 1, len(p.AllowedIngresses))
			assert.Equal(t, 1, len(p.AllowedIngresses[0].IPBlockFilters))
			assert.Equal(t, 0, len(p.AllowedIngresses[0].PortFilters))
			assert.Equal(t, p.AllowedIngresses[0].IPBlockFilters[0].Allow, np2.Spec.Ingress[0].From[0].IPBlock.CIDR)
			assert.True(t, stringInList(p.AllowedIngresses[0].IPBlockFilters[0].Block, np2.Spec.Ingress[0].From[0].IPBlock.Except[0]))
			assert.True(t, stringInList(p.AllowedIngresses[0].IPBlockFilters[0].Block, np2.Spec.Ingress[0].From[0].IPBlock.Except[1]))
		})

		anyNetworkPolicy(t, m.NetworkPolicies, "allow-everything", func(t *testing.T, p model.NetworkPolicy) {
			assert.Equal(t, 1, len(p.AllowedIngresses))
			assert.Equal(t, 0, len(p.AllowedIngresses[0].IPBlockFilters))
			assert.Equal(t, 0, len(p.AllowedIngresses[0].PortFilters))
		})

		anyNetworkPolicy(t, m.NetworkPolicies, "block-everything", func(t *testing.T, p model.NetworkPolicy) {
			assert.Equal(t, 0, len(p.AllowedIngresses))
		})

		anyNetworkPolicy(t, m.NetworkPolicies, "multiple-ports-with-ranges", func(t *testing.T, p model.NetworkPolicy) {
			assert.Equal(t, 1, len(p.AllowedIngresses))
			assert.Equal(t, 0, len(p.AllowedIngresses[0].IPBlockFilters))
			assert.Equal(t, 3, len(p.AllowedIngresses[0].PortFilters))
			assert.Equal(t, corev1.ProtocolTCP, p.AllowedIngresses[0].PortFilters[0].Protocol)
			assert.Equal(t, int32(2000), *p.AllowedIngresses[0].PortFilters[0].Port)
			assert.Equal(t, int32(3000), *p.AllowedIngresses[0].PortFilters[0].EndPort)
			assert.Equal(t, corev1.ProtocolUDP, p.AllowedIngresses[0].PortFilters[1].Protocol)
			assert.Equal(t, int32(4000), *p.AllowedIngresses[0].PortFilters[1].Port)
			assert.Equal(t, int32(5000), *p.AllowedIngresses[0].PortFilters[1].EndPort)
			assert.Equal(t, corev1.ProtocolUDP, p.AllowedIngresses[0].PortFilters[2].Protocol)
			assert.Equal(t, int32(6000), *p.AllowedIngresses[0].PortFilters[2].Port)
			assert.Equal(t, int32(7000), *p.AllowedIngresses[0].PortFilters[2].EndPort)
		})
	})
}

func TestNetworkPolicyWithoutIPBlock(t *testing.T) {
	f := newPodGeneratorFixture(t)

	np1 := newNetworkPolicy("allow-namespace")
	np1.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np1.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
		{
			From: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"project": "proj1",
						},
					},
				},
			},
		},
	}
	f.addNetworkPolicy(np1)

	np2 := newNetworkPolicy("allow-pod")
	np2.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np2.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
		{
			From: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "nginx",
						},
					},
				},
			},
		},
	}
	f.addNetworkPolicy(np2)

	np3 := newNetworkPolicy("allow-http-from-pod")
	np3.Spec.PolicyTypes = []networkingv1.PolicyType{
		"Ingress",
	}
	np3.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
		{
			From: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "ingress",
						},
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				newNetworkPolicyPort(corev1.ProtocolTCP, 80, 0),
			},
		},
	}
	f.addNetworkPolicy(np3)

	f.runWith(func(g *PodLoadBalancerModelGenerator) {
		m, err := g.GenerateModel(nil)

		assert.Nil(t, err)
		assert.NotNil(t, m)
		anyNetworkPolicy(t, m.NetworkPolicies, "allow-namespace", func(t *testing.T, p model.NetworkPolicy) {
			assert.Equal(t, 0, len(p.AllowedIngresses))
		})
		anyNetworkPolicy(t, m.NetworkPolicies, "allow-pod", func(t *testing.T, p model.NetworkPolicy) {
			assert.Equal(t, 0, len(p.AllowedIngresses))
		})
		anyNetworkPolicy(t, m.NetworkPolicies, "allow-http-from-pod", func(t *testing.T, p model.NetworkPolicy) {
			assert.Equal(t, 0, len(p.AllowedIngresses))
		})
	})
}
