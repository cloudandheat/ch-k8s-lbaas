package controller

import (
	"fmt"
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

	willAllowCleanups bool
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

	w := NewWorker(f.l3portmanager, f.portmapper, f.kubeclient, k8sI.Core().V1().Services().Lister())
	w.AllowCleanups = f.willAllowCleanups
	return w, k8sI
}

func (f *workerFixture) run(j WorkerJob) (*Worker, RequeueMode) {
	w, requeue, _ := f.runWithChecksAndEnv(j, true, false)
	return w, requeue
}

func (f *workerFixture) runExpectError(j WorkerJob) (*Worker, RequeueMode, error) {
	return f.runWithChecksAndEnv(j, true, true)
}

func (f *workerFixture) runWithChecksAndEnv(j WorkerJob, startInformers bool, expectError bool) (*Worker, RequeueMode, error) {
	var requeueMode RequeueMode
	var err error
	w := f.runWith(startInformers, func(w *Worker) {
		requeueMode, err = j.Run(w)
	})

	if !expectError && err != nil {
		f.t.Errorf("error syncing foo: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing foo, got nil")
	}

	return w, requeueMode, err
}

func (f *workerFixture) runWith(startInformers bool, body func(w *Worker)) *Worker {
	w, k8sI := f.newWorker()
	if startInformers {
		stopCh := make(chan struct{})
		defer close(stopCh)
		k8sI.Start(stopCh)
	}

	body(w)

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

	f.portmapper.AssertExpectations(f.t)
	f.l3portmanager.AssertExpectations(f.t)

	return w
}

func (f *workerFixture) expectCreateServiceAction(s *corev1.Service) {
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s))
}

func (f *workerFixture) expectUpdateServiceAction(s *corev1.Service) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s))
}

func (f *workerFixture) expectUpdateServiceStatusAction(s *corev1.Service) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateSubresourceAction(schema.GroupVersionResource{Resource: "services"}, "status", s.Namespace, s))
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

func TestSyncServiceAddsManagedAnnotationIfMissing(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	f.addService(s)
	j := &SyncServiceJob{model.FromService(s)}

	updatedS := s.DeepCopy()
	updatedS.Annotations = make(map[string]string)
	updatedS.Annotations[AnnotationManaged] = "true"
	f.expectUpdateServiceAction(updatedS)

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestSyncServiceRemovesManagedAnnotationAndUnmapsIfNotManageable(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Spec.Type = "not-a-load-balancer"
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	setPortAnnotation(s, "some-random-port")
	f.addService(s)

	f.portmapper.On("UnmapService", model.FromService(s)).Return(nil).Times(1)

	j := &SyncServiceJob{model.FromService(s)}

	updatedS := s.DeepCopy()
	updatedS.Annotations = make(map[string]string)

	f.expectUpdateServiceAction(updatedS)

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestSyncServiceIgnoresUnmanageableAndUnmanagedService(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Spec.Type = "not-a-load-balancer"
	f.addService(s)

	j := &SyncServiceJob{model.FromService(s)}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestSyncServiceDoesNotUpdateTheServiceIfNotLoadBalancer(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Spec.Type = "something-else"
	f.addService(s)

	j := &SyncServiceJob{model.FromService(s)}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestSyncServiceDoesNotUpdateTheServiceIfAnnotatedWithFalse(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "false"
	f.addService(s)

	j := &SyncServiceJob{model.FromService(s)}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestSyncServiceCallsMapServiceForManagedServiceAndAnnotatesPort(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	f.addService(s)

	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("random-port-id", nil).Times(1)

	updatedS := s.DeepCopy()
	setPortAnnotation(updatedS, "random-port-id")
	f.expectUpdateServiceAction(updatedS)

	j := &SyncServiceJob{model.FromService(s)}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestSyncServiceSetsLoadBalancerStatusOnUnchangedMapping(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	setPortAnnotation(s, "random-port-id")
	f.addService(s)

	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("random-port-id", nil).Times(1)
	f.l3portmanager.On("GetExternalAddress", "random-port-id").Return("some-ip", "some-hostname", nil).Times(1)

	j := &SyncServiceJob{model.FromService(s)}

	updatedS := s.DeepCopy()
	updatedS.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "some-ip", Hostname: "some-hostname"},
	}
	f.expectUpdateServiceStatusAction(updatedS)

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestSyncServiceRequeuesIfExternalAddressCannotBeObtained(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	setPortAnnotation(s, "random-port-id")
	f.addService(s)

	someError := fmt.Errorf("some error")
	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("random-port-id", nil).Times(1)
	f.l3portmanager.On("GetExternalAddress", "random-port-id").Return("", "", someError).Times(1)

	j := &SyncServiceJob{model.FromService(s)}

	_, requeue, err := f.runExpectError(j)
	assert.Equal(t, someError, err)
	assert.Equal(t, RequeueTail, requeue)
}

func TestSyncServiceClearsLoadBalancerStatusIfMappingHasChanged(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	setPortAnnotation(s, "old-port-id")
	s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "old-ip", Hostname: "bork-hostname"},
	}
	f.addService(s)

	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("random-port-id", nil).Times(1)

	updatedS := s.DeepCopy()
	updatedS.Status.LoadBalancer.Ingress = nil
	f.expectUpdateServiceStatusAction(updatedS)

	j := &SyncServiceJob{model.FromService(s)}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestSyncServiceRequeuesIfMapServiceFails(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	f.addService(s)

	someError := fmt.Errorf("some error")

	f.portmapper.On("MapService", s).Return(someError).Times(1)

	j := &SyncServiceJob{model.FromService(s)}

	_, requeue, err := f.runExpectError(j)
	assert.Equal(t, someError, err)
	assert.Equal(t, RequeueTail, requeue)
}

func TestSyncServiceDoesNothingIfDeleted(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	// not calling f.addService yields the same error as a deleted service

	j := &SyncServiceJob{model.FromService(s)}

	_, requeue, _ := f.runExpectError(j)
	assert.Equal(t, Drop, requeue)
}

func TestRemoveServiceUnmapsManagedService(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	// not calling f.addService yields the same error as a deleted service

	f.portmapper.On("UnmapService", model.FromService(s)).Return(nil).Times(1)

	j := &RemoveServiceJob{model.FromService(s), s.Annotations}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestRemoveServiceRetiresIfUnmappingFails(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	s.Annotations = make(map[string]string)
	s.Annotations["cah-loadbalancer.k8s.cloudandheat.com/managed"] = "true"
	// not calling f.addService yields the same error as a deleted service

	someError := fmt.Errorf("a random error")
	f.portmapper.On("UnmapService", model.FromService(s)).Return(someError).Times(1)

	j := &RemoveServiceJob{model.FromService(s), s.Annotations}

	_, requeue, err := f.runExpectError(j)
	assert.Equal(t, RequeueTail, requeue)
	assert.Equal(t, someError, err)
}

func TestRemoveServiceIgnoresUnmanagedService(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	// not calling f.addService yields the same error as a deleted service

	j := &RemoveServiceJob{model.FromService(s), s.Annotations}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestCleanupJobFailsWithRequeueIfBarrierIsInPlace(t *testing.T) {
	f := newWorkerFixture(t)

	j := &CleanupJob{}

	_, requeue, err := f.runExpectError(j)
	assert.Equal(t, RequeueTail, requeue)
	assert.Equal(t, ErrCleanupBarrierActive, err)
}

func TestCleanupJobAsksPortmanagerToRemoveUnusedPorts(t *testing.T) {
	f := newWorkerFixture(t)
	f.willAllowCleanups = true

	f.portmapper.On("GetUsedL3Ports").Return([]string{"a", "b"}, nil).Times(1)
	f.l3portmanager.On("CleanUnusedPorts", []string{"a", "b"}).Return(nil).Times(1)

	j := &CleanupJob{}

	_, requeue := f.run(j)
	assert.Equal(t, Drop, requeue)
}

func TestCleanupJobRetriesOnErrorFromPortmapper(t *testing.T) {
	f := newWorkerFixture(t)
	f.willAllowCleanups = true

	someError := fmt.Errorf("fnord")
	f.portmapper.On("GetUsedL3Ports").Return(nil, someError).Times(1)

	j := &CleanupJob{}

	_, requeue, err := f.runExpectError(j)
	assert.Equal(t, RequeueTail, requeue)
	assert.Equal(t, someError, err)
}

func TestCleanupJobRetriesOnErrorFromPortmanager(t *testing.T) {
	f := newWorkerFixture(t)
	f.willAllowCleanups = true

	someError := fmt.Errorf("fnord")
	f.portmapper.On("GetUsedL3Ports").Return([]string{"a", "b"}, nil).Times(1)
	f.l3portmanager.On("CleanUnusedPorts", []string{"a", "b"}).Return(someError).Times(1)

	j := &CleanupJob{}

	_, requeue, err := f.runExpectError(j)
	assert.Equal(t, RequeueTail, requeue)
	assert.Equal(t, someError, err)
}

func TestPmapServiceRemovesLBStatusAndReturnsTrueIfMappingChangesAndLBStatusIsPresent(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	setPortAnnotation(s, "old-port")
	s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "ip", Hostname: "hostname"}}
	f.addService(s)

	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("new-port", nil).Times(1)

	updatedS := s.DeepCopy()
	updatedS.Status.LoadBalancer.Ingress = nil
	f.expectUpdateServiceStatusAction(updatedS)

	f.runWith(true, func(w *Worker) {
		updated, err := w.mapService(s)
		assert.Nil(t, err)
		assert.True(t, updated)
	})
}

func TestPmapServiceDoesNothingAndReturnsFalseIfMappingIsUnchangedAndLBStatusIsPresent(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	setPortAnnotation(s, "old-port")
	s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "ip", Hostname: "hostname"}}
	f.addService(s)

	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("old-port", nil).Times(1)

	f.runWith(true, func(w *Worker) {
		updated, err := w.mapService(s)
		assert.Nil(t, err)
		assert.False(t, updated)
	})
}

func TestPmapServiceUpdatesAnnotationAndReturnsTrueIfMappingChangesAndLBStatusIsNotPresent(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	setPortAnnotation(s, "old-port")
	s.Status.LoadBalancer.Ingress = nil
	f.addService(s)

	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("new-port", nil).Times(1)

	updatedS := s.DeepCopy()
	setPortAnnotation(updatedS, "new-port")
	f.expectUpdateServiceAction(updatedS)

	f.runWith(true, func(w *Worker) {
		updated, err := w.mapService(s)
		assert.Nil(t, err)
		assert.True(t, updated)
	})
}

func TestPmapServiceUpdatesAnnotationAndReturnsTrueIfMappedAndLBStatusIsNotPresent(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	clearPortAnnotation(s)
	s.Status.LoadBalancer.Ingress = nil
	f.addService(s)

	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("new-port", nil).Times(1)

	updatedS := s.DeepCopy()
	setPortAnnotation(updatedS, "new-port")
	f.expectUpdateServiceAction(updatedS)

	f.runWith(true, func(w *Worker) {
		updated, err := w.mapService(s)
		assert.Nil(t, err)
		assert.True(t, updated)
	})
}

func TestPmapServiceForwardsErrorFromMapService(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	clearPortAnnotation(s)
	s.Status.LoadBalancer.Ingress = nil
	f.addService(s)

	someError := fmt.Errorf("some error")
	f.portmapper.On("MapService", s).Return(someError).Times(1)

	f.runWith(true, func(w *Worker) {
		updated, err := w.mapService(s)
		assert.Equal(t, someError, err)
		assert.False(t, updated)
	})
}

func TestPmapServiceForwardsErrorFromGetServiceL3Port(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	clearPortAnnotation(s)
	s.Status.LoadBalancer.Ingress = nil
	f.addService(s)

	someError := fmt.Errorf("some error")
	f.portmapper.On("MapService", s).Return(nil).Times(1)
	f.portmapper.On("GetServiceL3Port", model.FromService(s)).Return("", someError).Times(1)

	f.runWith(true, func(w *Worker) {
		updated, err := w.mapService(s)
		assert.Equal(t, someError, err)
		assert.False(t, updated)
	})
}

func TestPmapServiceRemovesLBStatusAndReturnsTrueIfUnmappedAndLBStatusIsPresent(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	clearPortAnnotation(s)
	s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "ip", Hostname: "hostname"}}
	f.addService(s)

	updatedS := s.DeepCopy()
	updatedS.Status.LoadBalancer.Ingress = nil
	f.expectUpdateServiceStatusAction(updatedS)

	f.runWith(true, func(w *Worker) {
		updated, err := w.mapService(s)
		assert.Nil(t, err)
		assert.True(t, updated)
	})
}

func TestPupdateServiceStatusSetsLBStatusFromPortAnnotationIfAbsent(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	setPortAnnotation(s, "some-port")
	s.Status.LoadBalancer.Ingress = nil
	f.addService(s)

	f.l3portmanager.On("GetExternalAddress", "some-port").Return("port-ip", "port-hostname", nil).Times(1)

	updatedS := s.DeepCopy()
	updatedS.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "port-ip", Hostname: "port-hostname"},
	}
	f.expectUpdateServiceStatusAction(updatedS)

	f.runWith(true, func(w *Worker) {
		updated, err := w.updateServiceStatus(s)
		assert.Nil(t, err)
		assert.True(t, updated)
	})
}

func TestPupdateServiceStatusSetsLBStatusFromPortAnnotationIfMismatching(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	setPortAnnotation(s, "some-port")
	s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "wrong-ip", Hostname: "port-hostname"},
	}
	f.addService(s)

	f.l3portmanager.On("GetExternalAddress", "some-port").Return("port-ip", "port-hostname", nil).Times(1)

	updatedS := s.DeepCopy()
	updatedS.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "port-ip", Hostname: "port-hostname"},
	}
	f.expectUpdateServiceStatusAction(updatedS)

	f.runWith(true, func(w *Worker) {
		updated, err := w.updateServiceStatus(s)
		assert.Nil(t, err)
		assert.True(t, updated)
	})
}

func TestPupdateServiceStatusSetsLBStatusFromPortAnnotationIfMoreThanOne(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	setPortAnnotation(s, "some-port")
	s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "port-ip", Hostname: "port-hostname"},
		{IP: "other-ip", Hostname: "other-hostname"},
	}
	f.addService(s)

	f.l3portmanager.On("GetExternalAddress", "some-port").Return("port-ip", "port-hostname", nil).Times(1)

	updatedS := s.DeepCopy()
	updatedS.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "port-ip", Hostname: "port-hostname"},
	}
	f.expectUpdateServiceStatusAction(updatedS)

	f.runWith(true, func(w *Worker) {
		updated, err := w.updateServiceStatus(s)
		assert.Nil(t, err)
		assert.True(t, updated)
	})
}

func TestPupdateServiceStatusReturnsFalseIfStatusIsUpToDate(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	setPortAnnotation(s, "some-port")
	s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "port-ip", Hostname: "port-hostname"},
	}
	f.addService(s)

	f.l3portmanager.On("GetExternalAddress", "some-port").Return("port-ip", "port-hostname", nil).Times(1)

	f.runWith(true, func(w *Worker) {
		updated, err := w.updateServiceStatus(s)
		assert.Nil(t, err)
		assert.False(t, updated)
	})
}

func TestPupdateServiceStatusForwardsErrorFromGetExternalAddress(t *testing.T) {
	f := newWorkerFixture(t)
	s := newService("test-service")
	setPortAnnotation(s, "some-port")
	s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "port-ip", Hostname: "port-hostname"},
	}
	f.addService(s)

	someError := fmt.Errorf("some error")
	f.l3portmanager.On("GetExternalAddress", "some-port").Return("", "", someError).Times(1)

	f.runWith(true, func(w *Worker) {
		updated, err := w.updateServiceStatus(s)
		assert.Equal(t, someError, err)
		assert.False(t, updated)
	})
}
