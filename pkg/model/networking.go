package model

import (
	corev1 "k8s.io/api/core/v1"
)

type L4Port struct {
	Protocol corev1.Protocol
	Port int32
}

type ServiceModel struct {
	L3PortID string
	Ports []L4Port
}

type L3Port struct {
	Allocations map[int32]string
}

func (p *L3Port) L4PortFree(pl4 L4Port) bool {
	_, inuse := p.Allocations[pl4.Port]
	return !inuse
}
