package agent

import (
	"fmt"
	"io"
	"sort"
	"text/template"

	corev1 "k8s.io/api/core/v1"

	"github.com/cloudandheat/cah-loadbalancer/pkg/config"
	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
)

var (
	nftablesTemplate = template.Must(template.New("nftables.conf").Parse(`
{{ $cfg := . }}
table {{ .FilterTableType }} {{ .FilterTableName }} {
	chain {{ .FilterForwardChainName }} {
		ct mark {{ $cfg.FWMarkBits | printf "0x%x" }} and {{ $cfg.FWMarkMask | printf "0x%x" }} accept;
	}
}

table ip {{ .NATTableName }} {
	chain {{ .NATPreroutingChainName }} {
{{ range $fwd := .Forwards }}
		ip daddr {{ $fwd.InboundIP }} {{ $fwd.Protocol }} dport {{ $fwd.InboundPort }} mark set {{ $cfg.FWMarkBits | printf "0x%x" }} and {{ $cfg.FWMarkMask | printf "0x%x" }} ct mark set meta mark dnat to numgen inc mod {{ $fwd.DestinationAddresses | len }} map {
{{- range $index, $daddr := $fwd.DestinationAddresses }}{{ $index }} : {{ $daddr }}, {{ end -}}
		} : {{ $fwd.DestinationPort }};
{{ end }}
	}

	chain {{ .NATPostroutingChainName }} {
		mark {{ $cfg.FWMarkBits | printf "0x%x" }} and {{ $cfg.FWMarkMask | printf "0x%x" }} masquerade;
	}
}
`))

	ErrProtocolNotSupported = fmt.Errorf("Protocol is not supported")
)

type nftablesForward struct {
	Protocol             string
	InboundIP            string
	InboundPort          int32
	DestinationAddresses []string
	DestinationPort      int32
}

type nftablesConfig struct {
	FilterTableType         string
	FilterTableName         string
	FilterForwardChainName  string
	NATTableName            string
	NATPostroutingChainName string
	NATPreroutingChainName  string
	FWMarkBits              uint32
	FWMarkMask              uint32
	Forwards                []nftablesForward
}

type NftablesGenerator struct {
	Cfg config.Nftables
}

func copyAddresses(in []string) []string {
	result := make([]string, len(in))
	copy(result, in)
	return result
}

func (g *NftablesGenerator) mapProtocol(k8sproto corev1.Protocol) (string, error) {
	switch k8sproto {
	case corev1.ProtocolTCP:
		return "tcp", nil
	case corev1.ProtocolUDP:
		return "udp", nil
	default:
		return "", ErrProtocolNotSupported
	}
}

func (g *NftablesGenerator) GenerateStructuredConfig(m *model.LoadBalancer) (*nftablesConfig, error) {
	result := &nftablesConfig{
		FilterTableName:         g.Cfg.FilterTableName,
		FilterTableType:         g.Cfg.FilterTableType,
		FilterForwardChainName:  g.Cfg.FilterForwardChainName,
		NATTableName:            g.Cfg.NATTableName,
		NATPostroutingChainName: g.Cfg.NATPostroutingChainName,
		NATPreroutingChainName:  g.Cfg.NATPreroutingChainName,
		FWMarkBits:              g.Cfg.FWMarkBits,
		FWMarkMask:              g.Cfg.FWMarkMask,
		Forwards:                []nftablesForward{},
	}

	for _, ingress := range m.Ingress {
		for _, port := range ingress.Ports {
			mappedProtocol, err := g.mapProtocol(port.Protocol)
			if err != nil {
				return nil, err
			}

			addrs := copyAddresses(port.DestinationAddresses)
			sort.Strings(addrs)

			result.Forwards = append(result.Forwards, nftablesForward{
				Protocol:             mappedProtocol,
				InboundIP:            ingress.Address,
				InboundPort:          port.InboundPort,
				DestinationAddresses: addrs,
				DestinationPort:      port.DestinationPort,
			})
		}
	}

	sort.SliceStable(result.Forwards, func(i, j int) bool {
		fwdA := &result.Forwards[i]
		fwdB := &result.Forwards[j]
		isLess := fwdA.InboundIP < fwdB.InboundIP
		if isLess {
			return true
		}
		if fwdA.InboundIP != fwdB.InboundIP {
			return false
		}

		return fwdA.InboundPort < fwdB.InboundPort
	})

	return result, nil
}

func (g *NftablesGenerator) WriteStructuredConfig(cfg *nftablesConfig, out io.Writer) error {
	return nftablesTemplate.Execute(out, cfg)
}

func (g *NftablesGenerator) GenerateConfig(m *model.LoadBalancer, out io.Writer) error {
	scfg, err := g.GenerateStructuredConfig(m)
	if err != nil {
		return err
	}
	return g.WriteStructuredConfig(scfg, out)
}
