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
package testing

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
	"github.com/stretchr/testify/mock"
)

// TODO: use mockery

type MockPortMapper struct {
	mock.Mock
}

type MockLoadBalancerModelGenerator struct {
	mock.Mock
}

type MockAgentController struct {
	mock.Mock
}

func softCastStringArray(something interface{}) []string {
	if something == nil {
		return nil
	}
	return something.([]string)
}

func softCastServiceIdentifierArray(something interface{}) []model.ServiceIdentifier {
	if something == nil {
		return nil
	}
	return something.([]model.ServiceIdentifier)
}

func NewMockPortMapper() *MockPortMapper {
	return new(MockPortMapper)
}

func (m *MockPortMapper) MapService(svc *corev1.Service) error {
	a := m.Called(svc)
	return a.Error(0)
}

func (m *MockPortMapper) UnmapService(id model.ServiceIdentifier) error {
	a := m.Called(id)
	return a.Error(0)
}

func (m *MockPortMapper) GetServiceL3Port(id model.ServiceIdentifier) (string, error) {
	a := m.Called(id)
	return a.String(0), a.Error(1)
}

func (m *MockPortMapper) GetModel() map[string]string {
	a := m.Called()
	tmp := a.Get(0)
	if tmp == nil {
		return nil
	}
	return tmp.(map[string]string)
}

func (m *MockPortMapper) GetUsedL3Ports() ([]string, error) {
	a := m.Called()
	return softCastStringArray(a.Get(0)), a.Error(1)
}

func (m *MockPortMapper) SetAvailableL3Ports(portIDs []string) ([]model.ServiceIdentifier, error) {
	a := m.Called(portIDs)
	return softCastServiceIdentifierArray(a.Get(0)), a.Error(1)
}

func NewMockLoadBalancerModelGenerator() *MockLoadBalancerModelGenerator {
	return new(MockLoadBalancerModelGenerator)
}

func (m *MockLoadBalancerModelGenerator) GenerateModel(portAssignment map[string]string) (*model.LoadBalancer, error) {
	a := m.Called(portAssignment)
	obj := a.Get(0)
	if obj == nil {
		return nil, a.Error(1)
	}
	return obj.(*model.LoadBalancer), a.Error(1)
}

func NewMockAgentController() *MockAgentController {
	return new(MockAgentController)
}

func (m *MockAgentController) PushConfig(cfg *model.LoadBalancer) error {
	a := m.Called(cfg)
	return a.Error(0)
}
