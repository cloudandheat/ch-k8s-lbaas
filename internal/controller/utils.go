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
	corev1 "k8s.io/api/core/v1"
)

const (
	AnnotationManaged     = "cah-loadbalancer.k8s.cloudandheat.com/managed"
	AnnotationInboundPort = "cah-loadbalancer.k8s.cloudandheat.com/inbound-port"
)

func isServiceManaged(svc *corev1.Service) bool {
	if svc.Annotations == nil {
		return false
	}
	val, ok := svc.Annotations[AnnotationManaged]
	if !ok {
		return false
	}
	return val == "true"
}

func canServiceBeManaged(svc *corev1.Service) bool {
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	if svc.Annotations == nil {
		return true
	}
	val, ok := svc.Annotations[AnnotationManaged]
	if !ok {
		return true
	}
	return val != "false"
}

func getPortAnnotation(svc *corev1.Service) string {
	if svc.Annotations == nil {
		return ""
	}
	return svc.Annotations[AnnotationInboundPort]
}

func setPortAnnotation(svc *corev1.Service, portID string) {
	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string)
	}
	svc.Annotations[AnnotationInboundPort] = portID
}

func clearPortAnnotation(svc *corev1.Service) {
	if svc.Annotations == nil {
		return
	}
	delete(svc.Annotations, AnnotationInboundPort)
}
