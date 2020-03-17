package testing

import (
	"github.com/stretchr/testify/mock"
)

// TODO: use mockery

type MockL3PortManager struct {
	mock.Mock
}

func NewMockL3PortManager() *MockL3PortManager {
	return new(MockL3PortManager)
}

func (m *MockL3PortManager) ProvisionPort() (string, error) {
	a := m.Called()
	return a.String(0), a.Error(1)
}

func (m *MockL3PortManager) CleanUnusedPorts(usedPorts []string) error {
	a := m.Called(usedPorts)
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
