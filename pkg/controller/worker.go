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

type MapServiceJob struct {
	Service model.ServiceIdentifier
}

func (j *MapServiceJob) Run(w *Worker) (RequeueMode, error) {
	svc, err := w.servicesLister.Services(j.Service.Namespace).Get(j.Service.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO: unmap service
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

	return Drop, fmt.Errorf("Service did not match any code branch")
}

func (j *MapServiceJob) ToString() string {
	return fmt.Sprintf("MapService(%q)", j.Service.ToKey())
}
