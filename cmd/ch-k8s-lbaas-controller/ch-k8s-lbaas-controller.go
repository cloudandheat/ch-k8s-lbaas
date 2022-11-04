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
// The code in this file is also based on:
// https://github.com/kubernetes/sample-controller
// which is Copyright 2017 The Kubernetes Authors under the Apache 2.0 License
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/controller"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/openstack"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/signals"
)

var (
	masterURL  string
	kubeconfig string
	configPath string
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*300)

	fileCfg, err := config.ReadControllerConfigFromFile(configPath)
	if err != nil {
		klog.Fatalf("Failed reading config: %s", err.Error())
	}
	config.FillControllerConfig(&fileCfg)

	osClient, err := openstack.NewClient(&fileCfg.OpenStack.Global)
	if err != nil {
		klog.Fatalf("Failed to connect to OpenStack: %s", err.Error())
	}

	l3portmanager, err := osClient.NewOpenStackL3PortManager(
		&fileCfg.OpenStack.Networking,
	)
	if err != nil {
		klog.Fatalf("Failed to create L3 port manager: %s", err.Error())
	}

	agentController, err := controller.NewHTTPAgentController(fileCfg.Agents)
	if err != nil {
		klog.Fatalf("Failed to configure agent controller: %s", err.Error())
	}

	servicesInformer := kubeInformerFactory.Core().V1().Services()
	nodesInformer := kubeInformerFactory.Core().V1().Nodes()
	endpointsInformer := kubeInformerFactory.Core().V1().Endpoints()
	networkPoliciesInformer := kubeInformerFactory.Networking().V1().NetworkPolicies()
	podsInformer := kubeInformerFactory.Core().V1().Pods() // TODO: I don't want to be informed about pods. Just need to list them

	modelGenerator, err := controller.NewLoadBalancerModelGenerator(
		fileCfg.BackendLayer,
		l3portmanager,
		servicesInformer.Lister(),
		nodesInformer.Lister(),
		endpointsInformer.Lister(),
		networkPoliciesInformer.Lister(),
		podsInformer.Lister(),
	)

	if fileCfg.BackendLayer != config.BackendLayerNodePort {
		// Setting the nodes informer to nil causes the controller not
		// to subscribe to it, saving cycles.
		nodesInformer = nil
	}

	if fileCfg.BackendLayer != config.BackendLayerPod {
		// Setting the endpoints informer to nil causes the controller
		// not to subscribe to it, saving cycles.
		endpointsInformer = nil
	}

	lbcontroller, err := controller.NewController(
		kubeClient,
		servicesInformer,
		nodesInformer,
		endpointsInformer,
		networkPoliciesInformer,
		l3portmanager,
		agentController,
		modelGenerator,
	)
	if err != nil {
		klog.Fatalf("Failed to configure controller: %s", err.Error())
	}

	http.Handle("/metrics", promhttp.Handler())

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", fileCfg.BindAddress, fileCfg.BindPort))
	if err != nil {
		klog.Fatalf("Failed to set up HTTP listener: %s", err.Error())
	}

	go http.Serve(listener, nil)

	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	kubeInformerFactory.Start(stopCh)

	if err = lbcontroller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&configPath, "config", "controller-config.toml", "Path to the controller config file.")
}
