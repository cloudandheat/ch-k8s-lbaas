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
// The code in this file is also heavily based on:
// https://github.com/kubernetes/sample-controller
// which is Copyright 2017 The Kubernetes Authors under the Apache 2.0 License
package controller

import (
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
)

const controllerAgentName = "cah-loadbalancer-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Foo is synced
	SuccessSynced = "Synced"

	SuccessTakenOver = "TakenOver"
	SuccessReleased  = "Released"

	MessageResourceTakenOver = "Service taken over by cah-loadbalancer-controller"
	MessageResourceReleased  = "Service released by cah-loadbalancer-controller"
)

// Controller is the controller implementation for Foo resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface

	servicesLister corelisters.ServiceLister
	servicesSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder

	worker *Worker
}

// NewController returns a new sample controller
func NewController(
	kubeclientset kubernetes.Interface,
	serviceInformer coreinformers.ServiceInformer,
	nodeInformer coreinformers.NodeInformer,
	endpointsInformer coreinformers.EndpointsInformer,
	networkPoliciesInformer networkinginformers.NetworkPolicyInformer,
	l3portmanager openstack.L3PortManager,
	agentController AgentController,
	generator LoadBalancerModelGenerator,
) (*Controller, error) {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	portmapper := NewPortMapper(l3portmanager)
	prometheus.DefaultRegisterer.MustRegister(
		NewCollector(portmapper),
	)

	controller := &Controller{
		kubeclientset:  kubeclientset,
		servicesLister: serviceInformer.Lister(),
		servicesSynced: serviceInformer.Informer().HasSynced,
		workqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Jobs"),
		recorder:       recorder,
		worker:         NewWorker(l3portmanager, portmapper, kubeclientset, serviceInformer.Lister(), generator, agentController),
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a Foo resource will enqueue that Foo resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			klog.Info("UpdateFunc called")
			/* if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			} */
			controller.handleObject(new)
		},
		DeleteFunc: controller.deleteObject,
	})

	if nodeInformer != nil {
		nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: controller.handleAuxUpdated,
			UpdateFunc: func(old, new interface{}) {
				oldNode := old.(*corev1.Node)
				newNode := new.(*corev1.Node)

				// addresses is all we care about
				if reflect.DeepEqual(oldNode.Status.Addresses, newNode.Status.Addresses) {
					return
				}

				controller.handleAuxUpdated(newNode)
			},
			DeleteFunc: controller.handleAuxUpdated,
		})
	}

	if endpointsInformer != nil {
		endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: controller.handleAuxUpdated,
			UpdateFunc: func(old, new interface{}) {
				oldEndpoints := old.(*corev1.Endpoints)
				newEndpoints := new.(*corev1.Endpoints)

				// addresses is all we care about
				if reflect.DeepEqual(oldEndpoints.Subsets, newEndpoints.Subsets) {
					return
				}

				controller.handleAuxUpdated(newEndpoints)
			},
			DeleteFunc: controller.handleAuxUpdated,
		})
	}

	if networkPoliciesInformer != nil {
		networkPoliciesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: controller.handleAuxUpdated,
			UpdateFunc: func(old, new interface{}) {
				klog.Info("UpdateFunc called")
				controller.handleAuxUpdated(new)
			},
			DeleteFunc: controller.handleAuxUpdated,
		})
	}

	return controller, nil
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Load Balancer controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.servicesSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	klog.Info("Informer caches are synchronized, enqueueing job to remove the cleanup barrier")

	klog.Info("Starting workers")
	go wait.Until(c.worker.Run, time.Second, stopCh)

	// 907s is chosen because:
	//
	// - it is prime, which randomizes the cleanups relative ordering against
	//   other jobs.
	// - it is greater than three times the sync interval of the informer
	//
	// This way, we can be reasonably confident that we have heard about all
	// currently active services before we attempt to clean any seemingly
	// unused ports.
	//
	// In the past, there have been problems with ports being deleted even
	// though they were still in use by services. This had been caused by the
	// cleanup job running too early, despite the fact that we block on the
	// informer sync above; apparently, the informer sync wait sometimes
	// returns too early or we are missing events for some other, unclear
	// reason.
	//
	// In order to mitigate this problem, we now ensure that the first cleanup
	// happens only after three sync intervals (900 seconds).
	go wait.Until(c.periodicCleanup, 907*time.Second, stopCh)

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *Controller) periodicCleanup() {
	// This is called "immediately" after the workers have started. We do not
	// want to schedule a cleanup immediately, though (observe the long comment
	// in the Run() function). Hence, we do not remove the cleanup barrier
	// there, but use the AllowCleanups flag here to decide whether it is the
	// first run.
	if c.worker.AllowCleanups {
		klog.Info("Triggering periodic cleanup")
		c.worker.EnqueueJob(&CleanupJob{})
	} else {
		// As this *is* the first run, we only remove the cleanup barrier; the
		// cleanup itself will be triggered on a subsequent run, after the
		// barrier has been removed by this job.
		klog.Info("Triggering removal of the cleanup barrier")
		c.worker.EnqueueJob(&RemoveCleanupBarrierJob{})
	}
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Foo resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Foo resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	klog.Infof("handleObject called with %T", obj)
	if object, ok = obj.(metav1.Object); !ok {
		klog.V(5).Infof("ignoring non-castable object in handleObject; expecting deletion event")
		return
	}
	klog.Infof("Processing object: %s/%s", object.GetNamespace(), object.GetName())

	// TODO: select only services which are meant for us; we will have to
	// add a configurable label/annotation to mark them.
	identifier, err := model.FromObject(object)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.worker.EnqueueJob(&SyncServiceJob{identifier})
}

func (c *Controller) deleteObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}

	identifier, err := model.FromObject(object)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	job := &RemoveServiceJob{}
	job.Service = identifier
	job.Annotations = make(map[string]string)
	for k, v := range object.GetAnnotations() {
		job.Annotations[k] = v
	}
	c.worker.EnqueueJob(job)
}

func (c *Controller) handleAuxUpdated(obj interface{}) {
	// FIXME: it would probably be good to filter this a little instead of just
	// updating on all changes.
	c.worker.EnqueueJob(&UpdateConfigJob{})
}
