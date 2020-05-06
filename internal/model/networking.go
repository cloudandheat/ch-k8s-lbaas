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
)

type L4Port struct {
	Protocol corev1.Protocol
	Port     int32
}

type ServiceModel struct {
	L3PortID string
	Ports    []L4Port
}

type L3Port struct {
	Allocations map[int32]string
}

func (p *L3Port) L4PortFree(pl4 L4Port) bool {
	_, inuse := p.Allocations[pl4.Port]
	return !inuse
}
