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
