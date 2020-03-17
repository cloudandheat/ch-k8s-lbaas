package agent

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"

	"github.com/cloudandheat/cah-loadbalancer/pkg/config"
	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
)

func newNftablesGenerator() *NftablesGenerator {
	cfg := &config.Nftables{}
	config.FillNftablesConfig(cfg)
	return &NftablesGenerator{
		Cfg: *cfg,
	}
}

func TestNftablesStructuredConfigFromEmptyLBModel(t *testing.T) {
	g := newNftablesGenerator()

	m := &model.LoadBalancer{
		Ingress: []model.IngressIP{},
	}

	scfg, err := g.GenerateStructuredConfig(m)
	assert.Nil(t, err)
	assert.NotNil(t, scfg)
	assert.Equal(t, g.Cfg.FilterTableName, scfg.FilterTableName)
	assert.Equal(t, g.Cfg.FilterTableType, scfg.FilterTableType)
	assert.Equal(t, g.Cfg.FilterForwardChainName, scfg.FilterForwardChainName)
	assert.Equal(t, g.Cfg.NATTableName, scfg.NATTableName)
	assert.Equal(t, g.Cfg.NATPostroutingChainName, scfg.NATPostroutingChainName)
	assert.Equal(t, g.Cfg.NATPreroutingChainName, scfg.NATPreroutingChainName)
	assert.Equal(t, g.Cfg.FWMarkBits, scfg.FWMarkBits)
	assert.Equal(t, g.Cfg.FWMarkMask, scfg.FWMarkMask)
	assert.NotNil(t, scfg.Forwards)
	assert.Equal(t, 0, len(scfg.Forwards))
}

func TestNftablesStructuredConfigFromNonEmptyLBModel(t *testing.T) {
	g := newNftablesGenerator()

	m := &model.LoadBalancer{
		Ingress: []model.IngressIP{
			{
				Address: "172.23.42.1",
				Ports: []model.PortForward{
					{
						InboundPort:          80,
						Protocol:             corev1.ProtocolTCP,
						DestinationPort:      30080,
						DestinationAddresses: []string{"192.168.0.1", "192.168.0.2"},
					},
					{
						InboundPort:          443,
						Protocol:             corev1.ProtocolTCP,
						DestinationPort:      30443,
						DestinationAddresses: []string{"192.168.0.1", "192.168.0.2"},
					},
				},
			},
			{
				Address: "172.23.42.2",
				Ports: []model.PortForward{
					{
						InboundPort:          53,
						Protocol:             corev1.ProtocolUDP,
						DestinationPort:      30053,
						DestinationAddresses: []string{"192.168.0.1", "192.168.0.2"},
					},
				},
			},
		},
	}

	scfg, err := g.GenerateStructuredConfig(m)
	assert.Nil(t, err)
	assert.NotNil(t, scfg)
	assert.NotNil(t, scfg.Forwards)
	assert.Equal(t, 3, len(scfg.Forwards))

	fwd := scfg.Forwards[0]
	assert.Equal(t, "tcp", fwd.Protocol)
	assert.Equal(t, m.Ingress[0].Address, fwd.InboundIP)
	assert.Equal(t, m.Ingress[0].Ports[0].InboundPort, fwd.InboundPort)
	assert.Equal(t, m.Ingress[0].Ports[0].DestinationPort, fwd.DestinationPort)
	assert.Equal(t, m.Ingress[0].Ports[0].DestinationAddresses, fwd.DestinationAddresses)

	fwd = scfg.Forwards[1]
	assert.Equal(t, "tcp", fwd.Protocol)
	assert.Equal(t, m.Ingress[0].Address, fwd.InboundIP)
	assert.Equal(t, m.Ingress[0].Ports[1].InboundPort, fwd.InboundPort)
	assert.Equal(t, m.Ingress[0].Ports[1].DestinationPort, fwd.DestinationPort)
	assert.Equal(t, m.Ingress[0].Ports[1].DestinationAddresses, fwd.DestinationAddresses)

	fwd = scfg.Forwards[2]
	assert.Equal(t, "udp", fwd.Protocol)
	assert.Equal(t, m.Ingress[1].Address, fwd.InboundIP)
	assert.Equal(t, m.Ingress[1].Ports[0].InboundPort, fwd.InboundPort)
	assert.Equal(t, m.Ingress[1].Ports[0].DestinationPort, fwd.DestinationPort)
	assert.Equal(t, m.Ingress[1].Ports[0].DestinationAddresses, fwd.DestinationAddresses)
}

func TestNftablesStructuredConfigSortsAddresses(t *testing.T) {
	g := newNftablesGenerator()

	m := &model.LoadBalancer{
		Ingress: []model.IngressIP{
			{
				Address: "172.23.42.3",
				Ports: []model.PortForward{
					{
						InboundPort:          443,
						Protocol:             corev1.ProtocolTCP,
						DestinationPort:      30443,
						DestinationAddresses: []string{"192.168.0.3", "192.168.0.2"},
					},
					{
						InboundPort:          80,
						Protocol:             corev1.ProtocolTCP,
						DestinationPort:      30080,
						DestinationAddresses: []string{"192.168.0.9", "192.168.0.2"},
					},
				},
			},
			{
				Address: "172.23.42.2",
				Ports: []model.PortForward{
					{
						InboundPort:          53,
						Protocol:             corev1.ProtocolUDP,
						DestinationPort:      30053,
						DestinationAddresses: []string{"192.168.0.1", "192.168.0.2"},
					},
				},
			},
		},
	}

	scfg, err := g.GenerateStructuredConfig(m)
	assert.Nil(t, err)
	assert.NotNil(t, scfg)
	assert.NotNil(t, scfg.Forwards)
	assert.Equal(t, 3, len(scfg.Forwards))

	fwd := scfg.Forwards[2]
	assert.Equal(t, "tcp", fwd.Protocol)
	assert.Equal(t, m.Ingress[0].Address, fwd.InboundIP)
	assert.Equal(t, m.Ingress[0].Ports[0].InboundPort, fwd.InboundPort)
	assert.Equal(t, m.Ingress[0].Ports[0].DestinationPort, fwd.DestinationPort)
	assert.Equal(t, []string{"192.168.0.2", "192.168.0.3"}, fwd.DestinationAddresses)

	fwd = scfg.Forwards[1]
	assert.Equal(t, "tcp", fwd.Protocol)
	assert.Equal(t, m.Ingress[0].Address, fwd.InboundIP)
	assert.Equal(t, m.Ingress[0].Ports[1].InboundPort, fwd.InboundPort)
	assert.Equal(t, m.Ingress[0].Ports[1].DestinationPort, fwd.DestinationPort)
	assert.Equal(t, []string{"192.168.0.2", "192.168.0.9"}, fwd.DestinationAddresses)

	fwd = scfg.Forwards[0]
	assert.Equal(t, "udp", fwd.Protocol)
	assert.Equal(t, m.Ingress[1].Address, fwd.InboundIP)
	assert.Equal(t, m.Ingress[1].Ports[0].InboundPort, fwd.InboundPort)
	assert.Equal(t, m.Ingress[1].Ports[0].DestinationPort, fwd.DestinationPort)
	assert.Equal(t, m.Ingress[1].Ports[0].DestinationAddresses, fwd.DestinationAddresses)
}
