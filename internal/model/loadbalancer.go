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
package model

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/golang-jwt/jwt"
)

type IPBlockFilter struct {
	Allow string   `json:"allow" validate:"cidr"`
	Block []string `json:"block" validate:"dive,cidr"`
}

type PortFilter struct {
	Protocol corev1.Protocol `json:"protocol" validate:"required,oneof=TCP UDP"`

	// Don't filter by port number if empty (only by protocol)
	Port    *int32 `json:"port,omitempty" validate:"required_with=EndPort,omitempty,gte=0,lte=65535"`
	EndPort *int32 `json:"end-port,omitempty" validate:"omitempty,gte=0,lte=65535,gtfield=Port"`
}

// AllowedIngress allows all incoming traffic by default.
// IPBlockFilters and PortFilters can by used to refine which traffic should be allowed
type AllowedIngress struct {
	// Don't filter by address if empty (allow all)
	IPBlockFilters []IPBlockFilter `json:"ipblock-filters" validate:"dive"`

	// Don't filter by transport protocol or port if empty (allow all)
	PortFilters []PortFilter `json:"port-filters" validate:"dive"`
}

// NetworkPolicy blocks all incoming traffic by default.
// Entries in AllowedIngress are used do allow certain (or all) traffic
type NetworkPolicy struct {
	Name string `json:"name" validate:"required"`

	// Block all incoming traffic if empty
	AllowedIngresses []AllowedIngress `json:"allowed-ingresses" validate:"dive"`
}

type PolicyAssignment struct {
	Address         string   `json:"address" validate:"required,ip"`
	NetworkPolicies []string `json:"network-policies" validate:"dive,required"`
}

type PortForward struct {
	Protocol             corev1.Protocol `json:"protocol" validate:"required,oneof=TCP UDP"`
	InboundPort          int32           `json:"inbound-port" validate:"gte=0,lte=65535"`
	DestinationAddresses []string        `json:"destination-addresses" validate:"required,dive,required,ip"`
	DestinationPort      int32           `json:"destination-port" validate:"gte=0,lte=65535"`
	BalancePolicy        string          `json:"policy"`
}

type IngressIP struct {
	Address string        `json:"address" validate:"ip"`
	Ports   []PortForward `json:"ports" validate:"dive"`
}

type LoadBalancer struct {
	Ingress           []IngressIP        `json:"ingress" validate:"dive"`
	NetworkPolicies   []NetworkPolicy    `json:"network-policies" validate:"dive"`
	PolicyAssignments []PolicyAssignment `json:"policy-assignments" validate:"dive"`
}

type ConfigClaim struct {
	Config LoadBalancer `json:"load-balancer-config" validate:"required"`
	jwt.StandardClaims
}
