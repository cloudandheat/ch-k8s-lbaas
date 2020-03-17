package agent

import (
	"io"
	"text/template"

	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
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
