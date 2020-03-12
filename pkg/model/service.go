package model

import (
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

var (
	ErrNotAValidKey     = errors.New("Not a valid namespace/name key")
)

type ServiceIdentifier struct {
	Namespace string
	Name      string
}

func FromService(svc *corev1.Service) ServiceIdentifier {
	return ServiceIdentifier{Namespace: svc.Namespace, Name: svc.Name}
}

func FromKey(key string) (ServiceIdentifier, error) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return ServiceIdentifier{}, ErrNotAValidKey
	}
	return ServiceIdentifier{Namespace: parts[0], Name: parts[1]}, nil
}

func (id ServiceIdentifier) ToKey() string {
	return fmt.Sprintf("%s/%s", id.Namespace, id.Name)
}
