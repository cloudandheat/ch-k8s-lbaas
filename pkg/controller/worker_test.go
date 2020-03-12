package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"

	"github.com/stretchr/testify/assert"

	controllertesting "github.com/cloudandheat/cah-loadbalancer/pkg/controller/testing"
	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
	ostesting "github.com/cloudandheat/cah-loadbalancer/pkg/openstack/testing"
)

type workerFixture struct {
	t *testing.T

	kubeclient    *k8sfake.Clientset
	serviceLister []*corev1.Service
	kubeactions   []core.Action
	kubeobjects   []runtime.Object

	l3portmanager *ostesting.MockL3PortManager
	portmapper    *controllertesting.MockPortMapper
}

func newWorkerFixture(t *testing.T) *workerFixture {
	f := &workerFixture{}
	f.t = t
	f.l3portmanager = ostesting.NewMockL3PortManager()
	f.portmapper = controllertesting.NewMockPortMapper()
	f.kubeobjects = []runtime.Object{}
	return f
}

func (f *workerFixture) newWorker() (*Worker, kubeinformers.SharedInformerFactory) {
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	for _, s := range f.serviceLister {
		k8sI.Core().V1().Services().Informer().GetIndexer().Add(s)
	}

	return NewWorker(f.l3portmanager, f.portmapper, f.kubeclient, k8sI.Core().V1().Services().Lister()), k8sI
}

func (f *workerFixture) run(j WorkerJob) (*Worker, RequeueMode) {
	w, requeue, _ := f.runWithChecksAndEnv(j, true, false)
	return w, requeue
}

func (f *workerFixture) runWithChecksAndEnv(j WorkerJob, startInformers bool, expectError bool) (*Worker, RequeueMode, error) {
	w, k8sI := f.newWorker()
	if startInformers {
		stopCh := make(chan struct{})
		defer close(stopCh)
		k8sI.Start(stopCh)
	}

	requeueMode, err := j.Run(w)
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

	return w, requeueMode, err
}

func (f *workerFixture) expectCreateServiceAction(s *corev1.Service) {
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s))
}

func (f *workerFixture) expectUpdateServiceAction(s *corev1.Service) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s))
}

func (f *workerFixture) addService(svc *corev1.Service) {
	f.serviceLister = append(f.serviceLister, svc)
	f.kubeobjects = append(f.kubeobjects, svc)
}

func TestWorkerInit(t *testing.T) {
	f := newWorkerFixture(t)
	w, _ := f.newWorker()
	assert.False(t, w.AllowCleanups)
}

func TestCleanupBarrierRemoval(t *testing.T) {
	f := newWorkerFixture(t)
	w, _ := f.newWorker()
	j := new(RemoveCleanupBarrierJob)

	requeue, err := j.Run(w)
	assert.Nil(t, err)
	assert.Equal(t, Drop, requeue)

	assert.True(t, w.AllowCleanups)
}

func TestMapServiceAddsManagedAnnotationIfMissing(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	f.addService(s)
	j := &MapServiceJob{model.FromService(s)}

	updatedS := s.DeepCopy()
	updatedS.Annotations = make(map[string]string)
	updatedS.Annotations[AnnotationManaged] = "true"
	f.expectUpdateServiceAction(updatedS)

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestMapServiceRemovesManagedAnnotationIfNotManageable(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Spec.Type = "not-a-load-balancer"
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	f.addService(s)

	j := &MapServiceJob{model.FromService(s)}

	updatedS := s.DeepCopy()
	updatedS.Annotations = make(map[string]string)

	f.expectUpdateServiceAction(updatedS)

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestMapServiceIgnoresUnmanageableAndUnmanagedService(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Spec.Type = "not-a-load-balancer"
	f.addService(s)

	j := &MapServiceJob{model.FromService(s)}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestMapServiceDoesNotUpdateTheServiceIfNotLoadBalancer(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Spec.Type = "something-else"
	f.addService(s)

	j := &MapServiceJob{model.FromService(s)}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestDoesNotUpdateTheServiceIfAnnotatedWithFalse(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "false"
	f.addService(s)

	j := &MapServiceJob{model.FromService(s)}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}
