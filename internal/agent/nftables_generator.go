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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"k8s.io/klog"

	corev1 "k8s.io/api/core/v1"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
)

var funcMap = template.FuncMap{
	// A very simplistic check if a string looks like an IPv4 address
	"isIPv4Address": func (ipString string) bool {
		return strings.Count(ipString, ":") == 0 && strings.Count(ipString, ".") == 3
	},
	// A very simplistic check if a string looks like an IPv6 address
	"isIPv6Address": func (ipString string) bool {
		return strings.Count(ipString, ":") >= 2
	},
	// Replace colons with dash
	"replaceColons": func (ipString string) string {
		return strings.ReplaceAll(ipString, ":", "-")
	},
}

var (
	nftablesTemplate = template.Must(template.New("nftables.conf").Funcs(funcMap).Parse(`
{{ $cfg := . }}

{{- if $cfg.PartialReload }}
# When partial reload is enabled, flush chains.
flush chain ip {{ .NATTableName }} {{ .NATPreroutingChainName }}
flush chain ip {{ .NATTableName }} {{ .NATPostroutingChainName }}

{{- if ne .FilterTableName "" }}
flush chain {{ .FilterTableType }} {{ .FilterTableName }} {{ .FilterForwardChainName }}

# Also delete all existing policy chains. 
# To prevent an error when the chain does not exists because of $reasons, create the chain before deleting it.
# This could be the case when the machine has been restarted (and therefore has "clean" nftables) and the last generated 
# lbaas nftables config is applied on start of the agent.
{{- range $chain := $cfg.ExistingPolicyChains }}
add chain {{ $cfg.FilterTableType }} {{ $cfg.FilterTableName }} {{ $chain }}
delete chain {{ $cfg.FilterTableType }} {{ $cfg.FilterTableName }} {{ $chain }}
{{- end }}
{{- end }}
{{- end }}

{{- if ne .FilterTableName "" }}
table {{ .FilterTableType }} {{ .FilterTableName }} {
	chain {{ .FilterForwardChainName }} {
		{{- range $dest := $cfg.PolicyAssignments }}
		ct mark {{ $cfg.FWMarkBits | printf "0x%x" }} and {{ $cfg.FWMarkMask | printf "0x%x" }} {{if isIPv4Address $dest.Address }}ip{{else if isIPv6Address $dest.Address}}ip6{{end}} daddr {{ $dest.Address }} goto {{ $cfg.PolicyPrefix }}POD-{{replaceColons $dest.Address}};
		{{- end }}
		ct mark {{ $cfg.FWMarkBits | printf "0x%x" }} and {{ $cfg.FWMarkMask | printf "0x%x" }} accept;
	}

	# Using uppercase POD to prevent collisions with policy names like 'pod-x.x.x.x'
	{{- range $pod := $cfg.PolicyAssignments }}
	chain {{ $cfg.PolicyPrefix }}POD-{{replaceColons $pod.Address}} {
		{{- range $pol := $pod.NetworkPolicies }}
		jump {{ $cfg.PolicyPrefix }}{{ $pol }};
		{{- end }}
		drop;
	}
	{{- end }}

	# Using uppercase RULE and CIDR to prevent collisions with policy names like 'x-rule-y-cidr-z'
	{{- range $policy := $cfg.NetworkPolicies }}
	chain {{ $cfg.PolicyPrefix }}{{ $policy.Name }} {
		{{- range $ruleIndex, $ingressRule := $policy.IngressRuleChains }}
		jump {{ $cfg.PolicyPrefix }}{{ $policy.Name }}-RULE{{ $ruleIndex }};
		{{- end }}
	}

	{{- range $ruleIndex, $ingressRule := $policy.IngressRuleChains }}
	chain {{ $cfg.PolicyPrefix }}{{ $policy.Name }}-RULE{{ $ruleIndex }} {
		{{- range $entryIndex, $entry := $ingressRule.Entries }}
		{{ $entry.SaddrMatch.Match }} {{ $entry.PortMatch }} {{- if ne ($entry.SaddrMatch.Except | len) 0 }} jump {{ $cfg.PolicyPrefix }}{{ $policy.Name }}-RULE{{ $ruleIndex }}-CIDR{{ $entryIndex }} {{- else }} accept {{- end }};

		{{- end }}
	}

	{{- range $entryIndex, $entry := $ingressRule.Entries }}
		{{- if ne ($entry.SaddrMatch.Except | len) 0 }}
	chain {{ $cfg.PolicyPrefix }}{{ $policy.Name }}-RULE{{ $ruleIndex }}-CIDR{{ $entryIndex }} {
		{{- range $addr := $entry.SaddrMatch.Except }}
		ip saddr {{ $addr }} return;
		{{- end}}
		accept;
	}
		{{- end }}
	{{- end }}

	{{- end }}

	{{- end }}
}
{{- end }}

table ip {{ .NATTableName }} {
	chain {{ .NATPreroutingChainName }} {
{{- range $fwd := .Forwards }}
{{- if ne ($fwd.DestinationAddresses | len) 0 }}
		ip daddr {{ $fwd.InboundIP }} {{ $fwd.Protocol }} dport {{ $fwd.InboundPort }} mark set {{ $cfg.FWMarkBits | printf "0x%x" }} and {{ $cfg.FWMarkMask | printf "0x%x" }} ct mark set meta mark dnat to numgen inc mod {{ $fwd.DestinationAddresses | len }} map {
{{- range $index, $daddr := $fwd.DestinationAddresses }}{{ $index }} : {{ $daddr }}, {{ end -}}
		} : {{ $fwd.DestinationPort }};
{{- end }}
{{- end }}
	}

{{- if $cfg.EnableSNAT }}
	chain {{ .NATPostroutingChainName }} {
		mark {{ $cfg.FWMarkBits | printf "0x%x" }} and {{ $cfg.FWMarkMask | printf "0x%x" }} masquerade;
	}
{{- end }}
}
`))

	ErrProtocolNotSupported = fmt.Errorf("Protocol is not supported")
)

type SAddrMatch struct {
	// String like eg. "ip saddr 0.0.0.0/0" ready to be used in an nftables rule.
	// May be "" so the rule doesn't match on source addresses (allow all)
	Match string

	// List of cidrs to block. If empty, set verdict to 'accept'.
	// If nonempty, set verdict to 'jump $c' and generate a new
	// chain $c that drops all address ranges and defaults to 'accept'
	Except []string
}

// Represents a single rule within an ingressRule chain.
type ingressRuleChainEntry struct {
	SaddrMatch SAddrMatch

	// String like eg. "tcp dport {80,443,8080-8090}" ready to be used in an nftables rule.
	// May be "" so the rule doesn't match on destination ports (allow all)
	PortMatch string
}

type ingressRuleChain struct {
	Entries []ingressRuleChainEntry
}

type networkPolicy struct {
	Name              string
	IngressRuleChains []ingressRuleChain
}

type policyAssignment struct {
	Address         string
	NetworkPolicies []string
}

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
	PolicyPrefix            string
	FWMarkBits              uint32
	FWMarkMask              uint32
	Forwards                []nftablesForward
	NetworkPolicies         map[string]networkPolicy
	PolicyAssignments       []policyAssignment
	ExistingPolicyChains    []string
	EnableSNAT              bool
	PartialReload           bool
}

type NftablesGenerator struct {
	Cfg config.Nftables
}

type nftablesChainListResultChain struct {
	Family string `json:"family"`
	Table  string `json:"table"`
	Name   string `json:"name"`
}

type nftablesChainListResultEntry struct {
	Chain nftablesChainListResultChain `json:"chain,omitempty"`
}

type nftablesChainListResult struct {
	Nftables []nftablesChainListResultEntry `json:"nftables"`
}

// Takes a slice of strings and produces a list ready to be used in an nftables rule
// eg. []string{"1", "2", "3"} => "{1,2,3}"
func makeNftablesList(items []string) (string, error) {
	if len(items) == 0 {
		return "", errors.New("Length of items must not be zero")
	}
	return "{" + strings.Join(items, ",") + "}", nil
}

//TODO: func to validate if a string is an ip address or cidr

// Generates a list of strings like "tcp dport match {80,443,8080-8090}"" ready to
// be used in nftables rules.
// returns []string{""} if 'in' is empty, so the rule doesn't match on
// destination ports (allow all)
func makePortMatches(in []model.PortFilter) (portMatches []string, err error) {
	portMap, err := makePortMap(in)
	if err != nil {
		return nil, err
	}
	portMatches = make([]string, 0, len(portMap)+1)
	for proto, ports := range portMap {
		nftablesList, err := makeNftablesList(ports)
		if err != nil {
			return nil, err
		}
		newMatch := proto + " dport " + nftablesList
		portMatches = append(portMatches, newMatch)
	}
	// if there are no port matches, all ports are allowed
	if len(portMatches) == 0 {
		portMatches = append(portMatches, "")
	}
	return portMatches, nil
}

// Generates a list of SAddrMatches to build nftable rules from
// returns a singleton with an empty SAddrMatch if 'in' is empty,
// so the rule doesn't match on ip addresses (allow all)
func makeSAddrMatches(in []model.IPBlockFilter) (SAddrMatches []SAddrMatch) {
	// if there are no IPBlockFilters, all address ranges are allowed
	if len(in) == 0 {
		return []SAddrMatch{
			{
				Match:  "",
				Except: []string{},
			},
		}
	}
	SAddrMatches = make([]SAddrMatch, 0, len(in))
	for _, block := range in {
		SAddrMatches = append(
			SAddrMatches, SAddrMatch{
				Match:  "ip saddr " + block.Allow,
				Except: copyAddresses(block.Block),
			},
		)
	}
	return SAddrMatches
}

func makeIngressRuleChain(rule model.AllowedIngress) (chain ingressRuleChain, err error) {
	portMatches, err := makePortMatches(rule.PortFilters)
	if err != nil {
		return chain, err
	}
	sAddrMatches := makeSAddrMatches(rule.IPBlockFilters)
	chain.Entries = make([]ingressRuleChainEntry, 0, len(sAddrMatches)*len(portMatches))
	for _, sAddrMatch := range sAddrMatches {
		for _, portMatch := range portMatches {
			newEntry := ingressRuleChainEntry{
				SaddrMatch: sAddrMatch,
				PortMatch:  portMatch,
			}
			chain.Entries = append(chain.Entries, newEntry)
		}
	}
	return chain, nil
}

func copyAddresses(in []string) []string {
	result := make([]string, len(in))
	copy(result, in)
	return result
}

// Takes a PortFilter and returns the protocol and a string representing the port or port range ready to be consumed by nftables.
// port==nil means all ports of the protocol are allowed
// eg. ("tcp", "80", nil), ("udp", nil, nil), ("tcp", "8080-8090", nil)
func makePortString(in model.PortFilter) (proto string, port *string, err error) {
	proto, err = mapProtocol(in.Protocol)
	if err != nil {
		return proto, port, err
	}
	if in.Port == nil {
		return
	} else {
		p := strconv.Itoa(int(*in.Port))
		port = &p
		if in.EndPort != nil {
			*port += "-" + strconv.Itoa(int(*in.EndPort)) // TODO: Use Join here?
		}
	}
	return proto, port, nil
}

// Takes a list of PortFilters and returns a map from protocol to port ranges.
// an empty list as value means that all ports of the protocol are allowed
// eg. map[string]{"tcp": []string{"80", "8080-8090"}, "udp": []string{"0-65535"}]}
// an empty map means that all ports of all protocols are allowed
func makePortMap(in []model.PortFilter) (portMap map[string][]string, err error) {
	portMap = make(map[string][]string)
	allowAll := make(map[string]bool)
	for _, port := range in {
		proto, portString, err := makePortString(port)
		if err != nil {
			return nil, err
		}
		if _, ok := portMap[proto]; !ok {
			portMap[proto] = make([]string, 0, 5)
		}
		if portString == nil {
			allowAll[proto] = true
		} else {
			portMap[proto] = append(portMap[proto], *portString)
		}
	}
	for proto, all := range allowAll {
		if all == true {
			portMap[proto] = []string{"0-65535"} // proto without port means all ports of that protocol are allowed
		}
	}
	return portMap, nil
}

func copyNetworkPolicies(in []model.NetworkPolicy) ([]networkPolicy, error) {
	result := make([]networkPolicy, len(in))
	var err error
	for i, policy := range in {
		result[i].Name = policy.Name
		result[i].IngressRuleChains = make([]ingressRuleChain, len(policy.AllowedIngresses))
		for j, rule := range policy.AllowedIngresses {
			result[i].IngressRuleChains[j], err = makeIngressRuleChain(rule)
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func copyPolicyAssignment(in []model.PolicyAssignment) []policyAssignment {
	result := make([]policyAssignment, len(in))
	for i, assignment := range in {
		result[i].Address = assignment.Address
		result[i].NetworkPolicies = copyAddresses(assignment.NetworkPolicies)
	}
	return result
}

// Maps from k8s.io/api/core/v1.Protocol objects to strings understood by nftables
func mapProtocol(k8sproto corev1.Protocol) (string, error) {
	switch k8sproto {
	case corev1.ProtocolTCP:
		return "tcp", nil
	case corev1.ProtocolUDP:
		return "udp", nil
	default:
		return "", ErrProtocolNotSupported
	}
}

// fetchNftablesChainList returns a result object with all nftables chains of type `tableType` using the `nftCommand`.
func fetchNftablesChainList(nftCommand []string, tableType string) (result nftablesChainListResult, err error) {
	// Prepare "list chains" command to get all chains of type tableType
	cmd := append(nftCommand, "-j", "list", "chains", tableType)

	klog.V(4).Infof("executing command: %#v", cmd)

	cmdObj := exec.Command(cmd[0], cmd[1:]...)
	cmdObj.Stderr = os.Stderr

	out, err := cmdObj.Output()
	if err != nil {
		return result, fmt.Errorf("failed to get exiting policy chains via %#v: %s", cmd, err.Error())
	}

	// Parse result from JSON
	err = json.Unmarshal(out, &result)
	if err != nil {
		return result, fmt.Errorf("could not parse existing policy json: %s", err.Error())
	}

	return result, nil
}

// filterNftablesChainListByPrefix filters a given chain list by `tableName`, `tableType` and `prefix`.
func filterNftablesChainListByPrefix(chains nftablesChainListResult, tableName string, tableType string, prefix string) (filteredChains []string, err error) {
	if prefix == "" {
		return filteredChains, fmt.Errorf("prefix must be set when filtering by prefix")
	}

	// Iterate over all returned chains and check if the conditions are met
	for _, resultEntry := range chains.Nftables {
		if resultEntry.Chain.Family == tableType &&
			resultEntry.Chain.Table == tableName &&
			strings.HasPrefix(resultEntry.Chain.Name, prefix) {
			// Append chain name to list
			filteredChains = append(filteredChains, resultEntry.Chain.Name)
		}
	}

	return filteredChains, nil
}

// getExistingPolicyChains returns all chain names of type `filterTableType` in table `filterTableName` that
// start with `policyPrefix`. Uses the `nftCommand` to retrieve the list via fetchNftablesChainList.
func getExistingPolicyChains(nftCommand []string, filterTableName string, filterTableType string, policyPrefix string) (existingChains []string, err error) {
	chains, err := fetchNftablesChainList(nftCommand, filterTableType)
	if err != nil {
		return existingChains, err
	}

	existingChains, err = filterNftablesChainListByPrefix(chains, filterTableName, filterTableType, policyPrefix)
	if err != nil {
		return existingChains, err
	}

	return existingChains, nil
}

// Generates a config suitable for nftablesTemplate from a LoadBalancer model
func (g *NftablesGenerator) GenerateStructuredConfig(m *model.LoadBalancer) (*nftablesConfig, error) {
	result := &nftablesConfig{
		FilterTableName:         g.Cfg.FilterTableName,
		FilterTableType:         g.Cfg.FilterTableType,
		FilterForwardChainName:  g.Cfg.FilterForwardChainName,
		NATTableName:            g.Cfg.NATTableName,
		NATPostroutingChainName: g.Cfg.NATPostroutingChainName,
		NATPreroutingChainName:  g.Cfg.NATPreroutingChainName,
		PolicyPrefix:            g.Cfg.PolicyPrefix,
		FWMarkBits:              g.Cfg.FWMarkBits,
		FWMarkMask:              g.Cfg.FWMarkMask,
		Forwards:                []nftablesForward{},
		NetworkPolicies:         map[string]networkPolicy{},
		PolicyAssignments:       []policyAssignment{},
		ExistingPolicyChains:    []string{},
		EnableSNAT:              g.Cfg.EnableSNAT,
		PartialReload:           g.Cfg.PartialReload,
	}

	for _, ingress := range m.Ingress {
		for _, port := range ingress.Ports {
			mappedProtocol, err := mapProtocol(port.Protocol)
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

	if g.Cfg.FilterTableName != "" {
		result.PolicyAssignments = copyPolicyAssignment(m.PolicyAssignments)
		policies, err := copyNetworkPolicies(m.NetworkPolicies)
		if err != nil {
			return nil, err
		}
		for _, policy := range policies {
			result.NetworkPolicies[policy.Name] = policy
		}

		if g.Cfg.PartialReload {
			// When partial reload is enabled, get all existing policy chain names to delete
			// them in the template
			result.ExistingPolicyChains, err = getExistingPolicyChains(
				g.Cfg.NftCommand,
				g.Cfg.FilterTableName,
				g.Cfg.FilterTableType,
				g.Cfg.PolicyPrefix)
		}
	}

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
