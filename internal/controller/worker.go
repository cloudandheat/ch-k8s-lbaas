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
	"context"
	goerrors "errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

type RequeueMode int

const (
	Drop        = RequeueMode(0)
	RequeueHead = RequeueMode(1)
	RequeueTail = RequeueMode(2)
)

const (
	EventServiceTakenOver              = "TakenOver"
	EventServiceReleased               = "Released"
	EventServiceMapped                 = "Mapped"
	EventServiceRemapped               = "Remapped"
	EventServiceAssigned               = "Assigned"
	EventServiceUnassignedForRemapping = "UnassignedForRemapping"
	EventServiceUnassignedStale        = "UnassignedStale"
	EventServiceUnmapped               = "Unmapped"

	MessageEventServiceTakenOver              = "Service taken over by cah-loadbalancer-controller"
	MessageEventServiceReleased               = "Service released by cah-loadbalancer-controller"
	MessageEventServiceMapped                 = "Service mapped to OpenStack port %q"
	MessageEventServiceAssigned               = "Service was assigned to the external IP address %q"
	MessageEventServiceUnassignedForRemapping = "Service was unassigned due to upcoming port remapping"
	MessageEventServiceUnassignedStale        = "Cleared stale IP address information"
	MessageEventServiceUnassignedDrop         = "Cleared IP address information because we release control over the Service"
	MessageEventServiceRemapped               = "Service mapping changed from port %q to %q (due to conflict)"
	MessageEventServiceUnmapped               = "Service unmapped"
)

var (
	ErrCleanupBarrierActive = goerrors.New("Cleanup barrier is in place")
)

type Worker struct {
	l3portmanager   openstack.L3PortManager
	portmapper      PortMapper
	servicesLister  corelisters.ServiceLister
	kubeclientset   kubernetes.Interface
	recorder        record.EventRecorder
	generator       LoadBalancerModelGenerator
	agentController AgentController

	workqueue workqueue.RateLimitingInterface

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

	_, err := w.kubeclientset.CoreV1().Services(svcSrc.Namespace).Update(context.TODO(), svc, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	w.recorder.Event(svc, corev1.EventTypeNormal, EventServiceTakenOver, MessageEventServiceTakenOver)
	return nil
}

func (w *Worker) releaseService(svcSrc *corev1.Service) error {
	if svcSrc.Status.LoadBalancer.Ingress != nil {
		// we clear the Ingress status first so that the annotations will serve
		// as a reminder that we need to do more cleanup, too
		svc := svcSrc.DeepCopy()
		svc.Status.LoadBalancer.Ingress = nil
		_, err := w.kubeclientset.CoreV1().Services(svcSrc.Namespace).UpdateStatus(context.TODO(), svc, metav1.UpdateOptions{})

		if err != nil {
			return err
		}

		w.recorder.Event(svc, corev1.EventTypeNormal, EventServiceUnassignedStale, MessageEventServiceUnassignedDrop)
		return nil
	}

	oldPortID := getPortAnnotation(svcSrc)
	w.portmapper.UnmapService(model.FromService(svcSrc))
	if oldPortID != "" {
		w.recorder.Event(svcSrc, corev1.EventTypeNormal, EventServiceUnmapped, MessageEventServiceUnmapped)
	}

	svc := svcSrc.DeepCopy()
	delete(svc.Annotations, AnnotationManaged)
	clearPortAnnotation(svc)

	klog.Infof("Releasing service %s/%s", svcSrc.Namespace, svcSrc.Name)

	_, err := w.kubeclientset.CoreV1().Services(svcSrc.Namespace).Update(context.TODO(), svc, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	w.recorder.Event(svc, corev1.EventTypeNormal, EventServiceReleased, MessageEventServiceReleased)
	return nil
}

// Map the service using the port mapper
//
// - Return true and no error if the resource was updated.
// - Return false and no error if the resource was not updated.
// - Return an error mapping the service or updating the resource failed and it
//   needs to be retired.
//
// This function will post the updated service to the k8s API.
func (w *Worker) mapService(svcSrc *corev1.Service) (updated bool, err error) {
	oldPortID := getPortAnnotation(svcSrc)
	if oldPortID == "" && svcSrc.Status.LoadBalancer.Ingress != nil {
		svc := svcSrc.DeepCopy()
		svc.Status.LoadBalancer.Ingress = nil
		_, err = w.kubeclientset.CoreV1().Services(svcSrc.Namespace).UpdateStatus(context.TODO(), svc, metav1.UpdateOptions{})
		w.recorder.Event(svc, corev1.EventTypeNormal, EventServiceUnassignedStale, MessageEventServiceUnassignedStale)
		return true, err
	}

	id := model.FromService(svcSrc)
	err = w.portmapper.MapService(svcSrc)
	if err != nil {
		return false, err
	}

	newPortID, err := w.portmapper.GetServiceL3Port(id)
	if err != nil {
		return false, err
	}

	if oldPortID != newPortID {
		svc := svcSrc.DeepCopy()
		if svc.Status.LoadBalancer.Ingress == nil {
			setPortAnnotation(svc, newPortID)
			_, err = w.kubeclientset.CoreV1().Services(svcSrc.Namespace).Update(context.TODO(), svc, metav1.UpdateOptions{})

			if oldPortID == "" {
				w.recorder.Event(svc, corev1.EventTypeNormal, EventServiceMapped, fmt.Sprintf(MessageEventServiceMapped, newPortID))
			} else {
				w.recorder.Event(svc, corev1.EventTypeNormal, EventServiceRemapped, fmt.Sprintf(MessageEventServiceRemapped, oldPortID, newPortID))
			}
		} else {
			svc.Status.LoadBalancer.Ingress = nil
			_, err = w.kubeclientset.CoreV1().Services(svcSrc.Namespace).UpdateStatus(context.TODO(), svc, metav1.UpdateOptions{})
			w.recorder.Event(svc, corev1.EventTypeNormal, EventServiceUnassignedForRemapping, MessageEventServiceUnassignedForRemapping)
		}

		return true, err
	}

	return false, err
}

// Update the load balancer status information
//
// - Return true and no error if the resource was updated.
// - Return false and no error if the resource was not updated.
// - Return an error retrieving the ingress information or updating the resource
//   failed and it needs to be retired.
//
// This function will post the updated status to the k8s API.
func (w *Worker) updateServiceStatus(svcSrc *corev1.Service) (updated bool, err error) {
	portID := getPortAnnotation(svcSrc)
	ipaddress, hostname, err := w.l3portmanager.GetExternalAddress(portID)
	if err != nil {
		return false, err
	}

	newIngress := corev1.LoadBalancerIngress{IP: ipaddress, Hostname: hostname}

	if len(svcSrc.Status.LoadBalancer.Ingress) != 1 ||
		svcSrc.Status.LoadBalancer.Ingress[0].Hostname != newIngress.Hostname ||
		svcSrc.Status.LoadBalancer.Ingress[0].IP != newIngress.IP {
		svc := svcSrc.DeepCopy()
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{newIngress}
		_, err = w.kubeclientset.CoreV1().Services(svcSrc.Namespace).UpdateStatus(context.TODO(), svc, metav1.UpdateOptions{})
		w.recorder.Event(svc, corev1.EventTypeNormal, EventServiceAssigned, fmt.Sprintf(MessageEventServiceAssigned, newIngress.IP))
		return true, err
	}

	return false, err
}

func (w *Worker) cleanupPorts() error {
	usedPorts, err := w.portmapper.GetUsedL3Ports()
	if err != nil {
		return err
	}

	err = w.l3portmanager.CleanUnusedPorts(usedPorts)
	if err != nil {
		return err
	}

	return nil
}

func NewWorker(
	l3portmanager openstack.L3PortManager,
	portmapper PortMapper,
	kubeclientset kubernetes.Interface,
	services corelisters.ServiceLister,
	generator LoadBalancerModelGenerator,
	agentController AgentController) *Worker {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	return &Worker{
		l3portmanager:   l3portmanager,
		portmapper:      portmapper,
		kubeclientset:   kubeclientset,
		servicesLister:  services,
		recorder:        recorder,
		generator:       generator,
		agentController: agentController,
		workqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Jobs"),
		AllowCleanups:   false,
	}
}

func (w *Worker) EnqueueJob(j WorkerJob) {
	w.workqueue.Add(j)
}

func (w *Worker) RequeueJob(j WorkerJob) {
	w.workqueue.AddRateLimited(j)
}

func (w *Worker) ShutDown() {
	w.workqueue.ShutDown()
}

func (w *Worker) Run() {
	klog.Infof("Worker started")
	for w.processNextJob() {
	}
}

func (w *Worker) executeJob(job WorkerJob) error {
	defer w.workqueue.Done(job)

	requeue, err := job.Run(w)
	if err != nil {
		if requeue != Drop {
			return fmt.Errorf(
				"error processing job %s: %s; requeueing",
				job.ToString(), err.Error(),
			)
		} else {
			return fmt.Errorf(
				"error processing job %s: %s; dropping",
				job.ToString(), err.Error(),
			)
		}
	}

	if requeue != Drop {
		w.workqueue.AddRateLimited(job)
	} else {
		w.workqueue.Forget(job)
	}

	klog.Infof("Successfully executed job %s", job.ToString())
	return nil
}

func (w *Worker) processNextJob() bool {
	jobInterface, shutdown := w.workqueue.Get()
	if shutdown {
		return false
	}

	job, ok := jobInterface.(WorkerJob)
	// We have to call workqueue.Done for all jobs, even those we Forget.
	// executeJob will call Done itself, so we don’t use defer here.
	if !ok {
		w.workqueue.Forget(job)
		w.workqueue.Done(job)
		utilruntime.HandleError(fmt.Errorf(
			"expected WorkerJob in queue, but got %#v", jobInterface,
		))
		return true
	}

	err := w.executeJob(job)
	if err != nil {
		utilruntime.HandleError(err)
	}

	return true
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

	// Issue:
	// We cannot update the Status of the service in the same API call as we
	// use to update the Metadata.
	//
	// At the same time, changing the Status changes the version of the Service,
	// which means that we cannot do the two calls in sequence without fetching
	// the Service in-between.
	//
	// Proposed flow:
	//
	// - input: service without port ID && without LB status
	//   -> map to port
	//   -> annotate port
	// - input: service with port ID && without LB status
	//   - if mapping does not change:
	//     -> set LB status
	//   - if mapping changes:
	//     -> update port annotation
	//     the next update will re-enter this branch until the mapping is
	//     constant
	// - input: service with port ID && with LB status
	//   - if mapping does not change:
	//     -> update LB status if needed
	//   - if mapping changes:
	//     -> clear LB status, leave annotation in place
	//     the next update will go into the
	//     "service with port ID && without LB status" branch which will change
	//     the mapping annotation and set the LB status
	//   benefit of this approach: the LB status is removed as soon as we know
	//   that it is going to be stale
	// - input: service without port ID && with LB status
	//   -> clear LB status
	//   the next update will deal with the mapping
	//
	// This is implemented by mapService refusing to do the update if there
	// is an LB annotation and the port mapping would change from the one
	// which is already on the resource; instead it removes the Ingress IP (and
	// returns true to indicate that it updated the resource).

	updated, err := w.mapService(svc)
	if err != nil {
		return RequeueTail, err
	}
	if updated {
		return Drop, nil
	}

	updated, err = w.updateServiceStatus(svc)
	if err != nil {
		return RequeueTail, err
	}

	// The work queue deduplicates jobs. In addition, the cleanup barrier will
	// prevent execution of the update config job (with requeue) so that no
	// harmful config will be generated during initial sync.
	w.EnqueueJob(&UpdateConfigJob{})
	return Drop, nil
}

func (j *SyncServiceJob) ToString() string {
	return fmt.Sprintf("SyncServiceJob(%q)", j.Service.ToKey())
}

type RemoveServiceJob struct {
	Service     model.ServiceIdentifier
	Annotations map[string]string
}

func (j *RemoveServiceJob) Run(w *Worker) (RequeueMode, error) {
	if j.Annotations == nil {
		return Drop, nil
	}
	if label, ok := j.Annotations[AnnotationManaged]; !ok || label != "true" {
		return Drop, nil
	}

	err := w.portmapper.UnmapService(j.Service)
	if err != nil {
		return RequeueTail, err
	}

	w.EnqueueJob(&CleanupJob{})
	w.EnqueueJob(&UpdateConfigJob{})
	return Drop, nil
}

func (j *RemoveServiceJob) ToString() string {
	return fmt.Sprintf("RemoveServiceJob(%q)", j.Service.ToKey())
}

type CleanupJob struct{}

func (j *CleanupJob) Run(w *Worker) (RequeueMode, error) {
	if !w.AllowCleanups {
		return RequeueTail, ErrCleanupBarrierActive
	}

	err := w.cleanupPorts()
	if err != nil {
		return RequeueTail, err
	}
	return Drop, nil
}

func (j *CleanupJob) ToString() string {
	return "CleanupJob"
}

type UpdateConfigJob struct{}

func (j *UpdateConfigJob) Run(w *Worker) (RequeueMode, error) {
	model, err := w.generator.GenerateModel(w.portmapper.GetModel())
	if err != nil {
		return RequeueTail, err
	}

	err = w.agentController.PushConfig(model)
	if err != nil {
		// TODO: should we post this as an event somewhere?
		return RequeueTail, err
	}

	return Drop, nil
}

func (j *UpdateConfigJob) ToString() string {
	return "UpdateConfigJob"
}
