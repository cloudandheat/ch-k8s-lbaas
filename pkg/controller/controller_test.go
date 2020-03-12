/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	ostesting "github.com/cloudandheat/cah-loadbalancer/pkg/openstack/testing"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	serviceLister []*corev1.Service
	// Actions expected to happen on the client.
	kubeactions []core.Action
	actions     []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
	objects     []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}
	return f
}

func (f *fixture) newController() (*Controller, kubeinformers.SharedInformerFactory) {
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	c, err := NewController(
		f.kubeclient,
		k8sI.Core().V1().Services(),
		ostesting.NewMockL3PortManager(),
	)
	if err != nil {
		klog.Fatalf("failed to construct controller: %s", err.Error())
	}

	c.servicesSynced = alwaysReady
	c.recorder = &record.FakeRecorder{}

	for _, s := range f.serviceLister {
		k8sI.Core().V1().Services().Informer().GetIndexer().Add(s)
	}

	return c, k8sI
}

func (f *fixture) run(serviceName string) {
	f.runController(serviceName, true, false)
}

func (f *fixture) runExpectError(serviceName string) {
	f.runController(serviceName, true, true)
}

func (f *fixture) runController(serviceName string, startInformers bool, expectError bool) {
	c, k8sI := f.newController()
	if startInformers {
		stopCh := make(chan struct{})
		defer close(stopCh)
		k8sI.Start(stopCh)
	}

	err := c.syncHandler(serviceName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing foo: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing foo, got nil")
	}

	k8sActions := filterInformerActions(f.kubeclient.Actions())
	for i, action := range k8sActions {
		if len(f.kubeactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeactions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeactions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.kubeactions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeactions)-len(k8sActions), f.kubeactions[len(k8sActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "services") ||
				action.Matches("watch", "services")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectCreateServiceAction(s *corev1.Service) {
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s))
}

func (f *fixture) expectUpdateServiceAction(s *corev1.Service) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s))
}

func newService(name string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}
}

func getKey(svc *corev1.Service, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(svc)
	if err != nil {
		t.Errorf("Unexpected error getting key for foo %v: %v", svc.Name, err)
		return ""
	}
	return key
}

func TestAddsManagedAnnotation(t *testing.T) {
	f := newFixture(t)
	s := newService("test-service")

	f.objects = append(f.objects, s)
	f.serviceLister = append(f.serviceLister, s)
	f.kubeobjects = append(f.kubeobjects, s)

	annotatedService := s.DeepCopy()
	annotatedService.Annotations = make(map[string]string)
	annotatedService.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"

	f.expectUpdateServiceAction(annotatedService)
	f.run(getKey(s, t))
}

func TestRemovesManagedAnnotationIfNotManageable(t *testing.T) {
	f := newFixture(t)
	s := newService("test-service")
	s.Spec.Type = "not-a-load-balancer"
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"

	f.objects = append(f.objects, s)
	f.serviceLister = append(f.serviceLister, s)
	f.kubeobjects = append(f.kubeobjects, s)

	patchedService := s.DeepCopy()
	patchedService.Annotations = make(map[string]string)

	f.expectUpdateServiceAction(patchedService)
	f.run(getKey(s, t))
}

func TestDoesNotUpdateTheServiceIfNotLoadBalancer(t *testing.T) {
	f := newFixture(t)
	s := newService("test-service")
	s.Spec.Type = "something-else"

	f.objects = append(f.objects, s)
	f.serviceLister = append(f.serviceLister, s)
	f.kubeobjects = append(f.kubeobjects, s)

	f.run(getKey(s, t))
}

func TestDoesNotUpdateTheServiceIfAnnotatedWithFalse(t *testing.T) {
	f := newFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "false"

	f.objects = append(f.objects, s)
	f.serviceLister = append(f.serviceLister, s)
	f.kubeobjects = append(f.kubeobjects, s)

	f.run(getKey(s, t))
}

func TestDoesNotUpdateTheServiceIfAnnotatedWithTrue(t *testing.T) {
	f := newFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "false"

	f.objects = append(f.objects, s)
	f.serviceLister = append(f.serviceLister, s)
	f.kubeobjects = append(f.kubeobjects, s)

	f.run(getKey(s, t))
}

func int32Ptr(i int32) *int32 { return &i }
