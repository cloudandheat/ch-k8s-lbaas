package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"

	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
	"github.com/cloudandheat/cah-loadbalancer/pkg/openstack"
)

type RequeueMode int

const (
	Drop        = RequeueMode(0)
	RequeueHead = RequeueMode(1)
	RequeueTail = RequeueMode(2)
)

type Worker struct {
	l3portmanager  openstack.L3PortManager
	portmapper     PortMapper
	servicesLister corelisters.ServiceLister
	kubeclientset  kubernetes.Interface

	AllowCleanups bool
}

func (w *Worker) takeOverService(svcSrc *corev1.Service) error {
	svc := svcSrc.DeepCopy()
	// TODO: find if there is a utility function to set an annotation
	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string)
	}
	svc.Annotations[AnnotationManaged] = "true"

	klog.Infof("Taking over service %s/%s", svcSrc.Namespace, svcSrc.Name)

	_, err := w.kubeclientset.CoreV1().Services(svcSrc.Namespace).Update(svc)
	if err != nil {
		return err
	}

	// TODO: re-add recorder use
	// w.recorder.Event(svc, corev1.EventTypeNormal, SuccessTakenOver, MessageResourceTakenOver)
	return nil
}

func (w *Worker) releaseService(svcSrc *corev1.Service) error {
	svc := svcSrc.DeepCopy()
	delete(svc.Annotations, AnnotationManaged)

	klog.Infof("Releasing service %s/%s", svcSrc.Namespace, svcSrc.Name)

	_, err := w.kubeclientset.CoreV1().Services(svcSrc.Namespace).Update(svc)
	if err != nil {
		return err
	}

	// TODO: re-add recorder use
	// w.recorder.Event(svc, corev1.EventTypeNormal, SuccessReleased, MessageResourceReleased)
	return nil
}

func NewWorker(
	l3portmanager openstack.L3PortManager,
	portmapper PortMapper,
	kubeclientset kubernetes.Interface,
	services corelisters.ServiceLister) *Worker {
	return &Worker{
		l3portmanager:  l3portmanager,
		portmapper:     portmapper,
		kubeclientset:  kubeclientset,
		servicesLister: services,
		AllowCleanups:  false,
	}
}

type WorkerJob interface {
	Run(w *Worker) (RequeueMode, error)
	ToString() string
}

type RemoveCleanupBarrierJob struct{}

func (j *RemoveCleanupBarrierJob) Run(state *Worker) (RequeueMode, error) {
	state.AllowCleanups = true
	return Drop, nil
}

func (j *RemoveCleanupBarrierJob) ToString() string {
	return "RemoveCleanupBarrier"
}


type SyncServiceJob struct {
	Service model.ServiceIdentifier
}

func (j *SyncServiceJob) Run(w *Worker) (RequeueMode, error) {
	svc, err := w.servicesLister.Services(j.Service.Namespace).Get(j.Service.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			// We do nothing here. We expect the listener to provide us with a
			// deleted event. The deleted event is handled differently.
			return Drop, err
		}
		return RequeueTail, err
	}

	isManaged := isServiceManaged(svc)
	canManage := canServiceBeManaged(svc)

	if !canManage {
		if isManaged {
			if err := w.releaseService(svc); err != nil {
				return RequeueTail, err
			}
		}
		return Drop, nil
	}

	if !isManaged {
		if err := w.takeOverService(svc); err != nil {
			return RequeueTail, err
		}
		return Drop, nil
	}

	err = w.portmapper.MapService(svc)
	if err != nil {
		return RequeueTail, err
	}
	// we have to update the annotation
	svcCopy := svc.DeepCopy()
	newPortID, err := w.portmapper.GetServiceL3Port(j.Service)
	if err != nil {
		return RequeueTail, err
	}
	setPortAnnotation(svcCopy, newPortID)

	_, err = w.kubeclientset.CoreV1().Services(svc.Namespace).Update(svcCopy)
	if err != nil {
		return RequeueTail, err
	}

	return Drop, nil
}

func (j *SyncServiceJob) ToString() string {
	return fmt.Sprintf("SyncServiceJob(%q)", j.Service.ToKey())
}


type RemoveServiceJob struct {
	Service model.ServiceIdentifier
	Annotations map[string]string
}

func (j *RemoveServiceJob) Run(w *Worker) (RequeueMode, error) {
	err := w.portmapper.UnmapService(j.Service)
	if err != nil {
		return RequeueTail, err
	}
	return Drop, nil
}

func (j *RemoveServiceJob) ToString() string {
	return fmt.Sprintf("RemoveServiceJob(%q)", j.Service.ToKey())
}
