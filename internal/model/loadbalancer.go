package model

import (
	corev1 "k8s.io/api/core/v1"

	jwt "github.com/dgrijalva/jwt-go"
)

type PortForward struct {
	Protocol             corev1.Protocol `json:"protocol"`
	InboundPort          int32           `json:"inbound-port"`
	DestinationAddresses []string        `json:"destination-addresses"`
	DestinationPort      int32           `json:"destination-port"`
	Policy               string          `json:"policy"`
}

type IngressIP struct {
	Address string        `json:"address"`
	Ports   []PortForward `json:"ports"`
}

type LoadBalancer struct {
	Ingress []IngressIP `json:"ingress"`
}

type ConfigClaim struct {
	Config LoadBalancer `json:"load-balancer-config"`
	jwt.StandardClaims
}
