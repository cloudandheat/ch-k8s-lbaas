package controller

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	AnnotationManaged = "cah-loadbalancer.k8s.cloudandheat.com/managed"
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
