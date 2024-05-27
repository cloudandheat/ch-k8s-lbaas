package openstack

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	portsv2 "github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	ostesting "github.com/cloudandheat/ch-k8s-lbaas/internal/openstack/testing"
)

type fixture struct {
	t *testing.T

	pm                   *OpenStackL3PortManager
	client               *ostesting.MockPortClient
	agents               []config.Agent
	expectedAddressPairs []portsv2.AddressPair
	l3Ports              []portsv2.Port
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{
		t:      t,
		client: &ostesting.MockPortClient{},
	}
	portIDs := []string{"gw-1-port-id", "gw-2-port-id", "gw-3-port-id"}

	f.agents = make([]config.Agent, len(portIDs))
	for i, portID := range portIDs {
		f.agents[i] = config.Agent{PortId: portID}
	}

	f.pm = &OpenStackL3PortManager{
		agents: f.agents,
		ports:  f.client,
		cfg:    &config.NetworkingOpts{},
	}

	// add sample setup for agents and L3 ports
	l3Ip1 := "10.0.0.1"
	l3Ip2 := "10.0.0.2"
	l3Ip3 := "10.0.0.3"
	additionalIp1 := "10.0.0.4"
	additionalIp2 := "10.0.0.5"

	f.pm.additionalAddressPairs = []string{additionalIp1, additionalIp2}

	f.expectedAddressPairs = []portsv2.AddressPair{
		{IPAddress: l3Ip1},
		{IPAddress: l3Ip2},
		{IPAddress: l3Ip3},
		{IPAddress: additionalIp1},
		{IPAddress: additionalIp2},
	}

	f.l3Ports = []portsv2.Port{
		{FixedIPs: []portsv2.IP{{IPAddress: l3Ip1}}},
		{FixedIPs: []portsv2.IP{{IPAddress: l3Ip2}, {IPAddress: l3Ip3}}},
	}

	return f
}

func getMatchIpFn(expectedAddressPairs []portsv2.AddressPair) func(opts portsv2.UpdateOpts) bool {
	matchIpsFn := func(opts portsv2.UpdateOpts) bool {
		addedIps := map[string]bool{}
		for _, ip := range expectedAddressPairs {
			addedIps[ip.IPAddress] = false
		}

		for _, ap := range *opts.AllowedAddressPairs {
			if _, expected := addedIps[ap.IPAddress]; !expected {
				fmt.Println(fmt.Errorf("Update call was invoked with unexpected ip address %v in to allowed address pairs.", ap.IPAddress))
				return false
			}
			addedIps[ap.IPAddress] = true
		}

		for ip, added := range addedIps {
			if !added {
				fmt.Println(fmt.Errorf("Did not add ip address to allowed address pairs %v", ip))
				return false
			}
		}

		return true
	}
	return matchIpsFn
}

func TestEnsureAgentsStateUpdateAddressPairsCorrectly(t *testing.T) {
	f := newFixture(t)

	f.client.On("GetPorts").Return(f.l3Ports, nil).Times(1)

	for _, agent := range f.agents {
		f.client.On("Update", mock.Anything, agent.PortId, mock.MatchedBy(getMatchIpFn(f.expectedAddressPairs))).Return(&portsv2.Port{}, nil).Times(1)
	}

	err := f.pm.EnsureAgentsState()
	assert.Nil(t, err)
	f.client.AssertExpectations(t)
}

func TestEnsureAgentsStateReturnsErrorIfPortsCannotBeFetched(t *testing.T) {
	f := newFixture(t)

	f.client.On("GetPorts").Return([]portsv2.Port{}, errors.New(""))
	err := f.pm.EnsureAgentsState()
	assert.NotNil(t, err)

	f.client.AssertExpectations(t)
}

func TestEnsureAgentsStateReturnsErrorIfUpdateFails(t *testing.T) {
	f := newFixture(t)

	f.client.On("GetPorts").Return(f.l3Ports, nil).Times(1)

	for i, agent := range f.agents {
		var returnErr error = nil
		if i == 0 {
			returnErr = errors.New("")
		}

		f.client.On("Update", mock.Anything, agent.PortId, mock.MatchedBy(getMatchIpFn(f.expectedAddressPairs))).Return(&portsv2.Port{}, returnErr).Times(1)
	}

	err := f.pm.EnsureAgentsState()
	assert.NotNil(t, err)
	f.client.AssertExpectations(t)
}
