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
	"github.com/gophercloud/gophercloud"
	"github.com/stretchr/testify/mock"

	floatingipsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	portsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
)

// TODO: use mockery

type MockL3PortManager struct {
	mock.Mock
}

type MockPortClient struct {
	mock.Mock
}

func NewMockL3PortManager() *MockL3PortManager {
	return new(MockL3PortManager)
}

func (m *MockL3PortManager) CheckPortExists(portID string) (bool, error) {
	a := m.Called(portID)
	return a.Bool(0), a.Error(1)
}

func (m *MockL3PortManager) ProvisionPort() (string, error) {
	a := m.Called()
	return a.String(0), a.Error(1)
}

func (m *MockL3PortManager) CleanUnusedPorts(usedPorts []string) error {
	a := m.Called(usedPorts)
	return a.Error(0)
}

func (m *MockL3PortManager) EnsureAgentsState() error {
	a := m.Called()
	return a.Error(0)
}

func (m *MockL3PortManager) GetAvailablePorts() ([]string, error) {
	a := m.Called()
	return a.Get(0).([]string), a.Error(1)
}

func (m *MockL3PortManager) GetExternalAddress(portID string) (string, string, error) {
	a := m.Called(portID)
	return a.String(0), a.String(1), a.Error(2)
}

func (m *MockL3PortManager) GetInternalAddress(portID string) (string, error) {
	a := m.Called(portID)
	return a.String(0), a.Error(1)
}

func (mpc *MockPortClient) Create(c *gophercloud.ServiceClient, opts portsv2.CreateOptsBuilder) (*portsv2.Port, error) {
	a := mpc.Called(c, opts)
	return a.Get(0).(*portsv2.Port), a.Error(1)
}

func (mpc *MockPortClient) GetPorts() ([]portsv2.Port, error) {
	a := mpc.Called()
	return a.Get(0).([]portsv2.Port), a.Error(1)
}

func (mpc *MockPortClient) GetPortByID(ID string) (*portsv2.Port, *floatingipsv2.FloatingIP, error) {
	a := mpc.Called(ID)
	return a.Get(0).(*portsv2.Port), a.Get(1).(*floatingipsv2.FloatingIP), a.Error(2)
}

func (mpc *MockPortClient) Update(c *gophercloud.ServiceClient, id string, opts portsv2.UpdateOptsBuilder) (*portsv2.Port, error) {
	a := mpc.Called(c, id, opts)
	return a.Get(0).(*portsv2.Port), a.Error(1)
}

func (mpc *MockPortClient) Delete(c *gophercloud.ServiceClient, id string) (r portsv2.DeleteResult) {
	a := mpc.Called(c, id)
	return a.Get(0).(portsv2.DeleteResult)
}
