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
	"io"
	"sort"
	"text/template"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
)

type ConfigGenerator interface {
	GenerateConfig(lb *model.LoadBalancer, out io.Writer) error
}

var (
	keepalivedTemplate = template.Must(template.New("keepalived.conf").Parse(`
{{ range .Instances }}
vrrp_instance LBaaS_{{ .Name }} {
    state BACKUP
    interface {{ .Interface }}
    virtual_router_id {{ .VRID }}
    priority {{ .Priority }}
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass {{ .Password }}
    }
    virtual_ipaddress {
{{ range .Addresses }}        {{ .Address }}/32 dev {{ .Device }}
{{ end }}    }
}
{{ end }}
`))
)

type keepalivedVRRPAddress struct {
	Address string
	Device  string
}

type keepalivedVRRPInstance struct {
	Name      string
	Interface string
	Priority  int
	VRID      int
	Password  string
	Addresses []keepalivedVRRPAddress
}

type keepalivedConfig struct {
	Instances []keepalivedVRRPInstance
}

type KeepalivedConfigGenerator struct {
	Priority     int
	VRRPPassword string
	VRIDBase     int
	Interface    string
}

func (g *KeepalivedConfigGenerator) GenerateStructuredConfig(lb *model.LoadBalancer) (*keepalivedConfig, error) {
	if len(lb.Ingress) == 0 {
		return &keepalivedConfig{
			Instances: []keepalivedVRRPInstance{},
		}, nil
	}

	result := &keepalivedConfig{
		Instances: []keepalivedVRRPInstance{
			{
				Name:      "VIPs",
				Interface: g.Interface,
				Priority:  g.Priority,
				VRID:      g.VRIDBase,
				Password:  g.VRRPPassword,
				Addresses: []keepalivedVRRPAddress{},
			},
		},
	}

	instance := &result.Instances[0]
	for _, ingress := range lb.Ingress {
		instance.Addresses = append(instance.Addresses, keepalivedVRRPAddress{
			Address: ingress.Address,
			Device:  g.Interface,
		})
	}

	sort.SliceStable(instance.Addresses, func(i, j int) bool {
		// TODO: if we ever switch to a multi interface setup, we’ll have to
		// take the interface into account, too.
		aA := instance.Addresses[i]
		aB := instance.Addresses[j]
		return aA.Address < aB.Address
	})

	return result, nil
}

func (g *KeepalivedConfigGenerator) WriteStructuredConfig(cfg *keepalivedConfig, out io.Writer) error {
	return keepalivedTemplate.Execute(out, cfg)
}

func (g *KeepalivedConfigGenerator) GenerateConfig(lb *model.LoadBalancer, out io.Writer) error {
	scfg, err := g.GenerateStructuredConfig(lb)
	if err != nil {
		return err
	}
	return g.WriteStructuredConfig(scfg, out)
}
