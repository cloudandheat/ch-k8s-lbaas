package controller

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	AnnotationManaged = "cah-loadbalancer.k8s.cloudandheat.com/managed"
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
