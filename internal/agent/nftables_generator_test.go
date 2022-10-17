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
package agent

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"

	"os"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
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
					{
						InboundPort:          8888,
						Protocol:             corev1.ProtocolTCP,
						DestinationPort:      38888,
						DestinationAddresses: []string{},
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
		NetworkPolicies: []model.NetworkPolicy{
			{
				Name: "allow-http",
				AllowedIngresses: []model.AllowedIngress{
					{
						PortFilters: []model.PortFilter{
							{
								Protocol: corev1.ProtocolTCP,
								Port:     func(i int32) *int32 { return &i }(80),
							},
							{
								Protocol: corev1.ProtocolTCP,
								Port:     func(i int32) *int32 { return &i }(8080),
								EndPort:  func(i int32) *int32 { return &i }(8090),
							},
						},
					},
				},
			},
			{
				Name: "block-range",
				AllowedIngresses: []model.AllowedIngress{
					{
						IPBlockFilters: []model.IPBlockFilter{
							{
								Allow: "0.0.0.0/0",
								Block: []string{
									"192.168.2.0/24",
									"192.168.178.0/24",
								},
							},
						},
					},
				},
			},
			{
				Name: "allow-udp",
				AllowedIngresses: []model.AllowedIngress{
					{
						PortFilters: []model.PortFilter{
							{
								Protocol: corev1.ProtocolUDP,
							},
						},
					},
				},
			},
		},
	}

	scfg, err := g.GenerateStructuredConfig(m)
	assert.Nil(t, err)
	assert.NotNil(t, scfg)
	assert.NotNil(t, scfg.Forwards)
	assert.Equal(t, 4, len(scfg.Forwards))

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
	assert.Equal(t, "tcp", fwd.Protocol)
	assert.Empty(t, fwd.DestinationAddresses)
	// Look at the generated template if needed (`go test -v`)
	g.WriteStructuredConfig(scfg, os.Stdout)

	fwd = scfg.Forwards[3]
	assert.Equal(t, "udp", fwd.Protocol)
	assert.Equal(t, m.Ingress[1].Address, fwd.InboundIP)
	assert.Equal(t, m.Ingress[1].Ports[0].InboundPort, fwd.InboundPort)
	assert.Equal(t, m.Ingress[1].Ports[0].DestinationPort, fwd.DestinationPort)
	assert.Equal(t, m.Ingress[1].Ports[0].DestinationAddresses, fwd.DestinationAddresses)

	pol := scfg.NetworkPolicies["allow-http"]
	assert.Equal(t, "allow-http", pol.Name)
	assert.Equal(t, 1, len(pol.IngressRuleChains))
	assert.Equal(t, 1, len(pol.IngressRuleChains[0].Entries))
	assert.Equal(t, "", pol.IngressRuleChains[0].Entries[0].SaddrMatch.Match)
	assert.Equal(t, 0, len(pol.IngressRuleChains[0].Entries[0].SaddrMatch.Except))
	assert.Equal(t, "tcp dport {80,8080-8090}", pol.IngressRuleChains[0].Entries[0].PortMatch)

	pol = scfg.NetworkPolicies["allow-udp"]
	assert.Equal(t, "allow-udp", pol.Name)
	assert.Equal(t, 1, len(pol.IngressRuleChains))
	assert.Equal(t, 1, len(pol.IngressRuleChains[0].Entries))
	assert.Equal(t, "", pol.IngressRuleChains[0].Entries[0].SaddrMatch.Match)
	assert.Equal(t, 0, len(pol.IngressRuleChains[0].Entries[0].SaddrMatch.Except))
	assert.Equal(t, "udp", pol.IngressRuleChains[0].Entries[0].PortMatch)

	pol = scfg.NetworkPolicies["block-range"]
	assert.Equal(t, "block-range", pol.Name)
	assert.Equal(t, 1, len(pol.IngressRuleChains))
	assert.Equal(t, 1, len(pol.IngressRuleChains[0].Entries))
	assert.Equal(t, "ip saddr 0.0.0.0/0", pol.IngressRuleChains[0].Entries[0].SaddrMatch.Match)
	assert.Equal(t, 2, len(pol.IngressRuleChains[0].Entries[0].SaddrMatch.Except))
	assert.Equal(t, "192.168.2.0/24", pol.IngressRuleChains[0].Entries[0].SaddrMatch.Except[0])
	assert.Equal(t, "192.168.178.0/24", pol.IngressRuleChains[0].Entries[0].SaddrMatch.Except[1])
	assert.Equal(t, "", pol.IngressRuleChains[0].Entries[0].PortMatch)
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
