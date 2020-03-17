package agent

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudandheat/cah-loadbalancer/pkg/model"
)

func newKeepalivedGenerator() *KeepalivedConfigGenerator {
	return &KeepalivedConfigGenerator{
		Priority:     23,
		VRRPPassword: "password",
		VRIDBase:     10,
		Interface:    "ethfoo",
	}
}

func TestGenerateStructuredConfigFromEmptyLBModel(t *testing.T) {
	g := newKeepalivedGenerator()

	m := &model.LoadBalancer{
		Ingress: []model.IngressIP{},
	}

	scfg, err := g.GenerateStructuredConfig(m)
	assert.Nil(t, err)
	assert.NotNil(t, scfg)
	assert.Equal(t, 0, len(scfg.Instances))
}

func TestGenerateStructuredConfigFromNonEmptyLBModel(t *testing.T) {
	g := newKeepalivedGenerator()

	m := &model.LoadBalancer{
		Ingress: []model.IngressIP{
			{
				Address: "127.0.0.1",
			},
			{
				Address: "127.0.0.2",
			},
			{
				Address: "127.0.0.3",
			},
		},
	}

	scfg, err := g.GenerateStructuredConfig(m)
	assert.Nil(t, err)
	assert.NotNil(t, scfg)
	assert.Equal(t, 1, len(scfg.Instances))
}

func TestGenerateConfigFromNonEmptyLBModel(t *testing.T) {
	g := newKeepalivedGenerator()

	m := &model.LoadBalancer{
		Ingress: []model.IngressIP{
			{
				Address: "127.0.0.1",
			},
			{
				Address: "127.0.0.2",
			},
			{
				Address: "127.0.0.3",
			},
		},
	}

	out := bytes.NewBuffer([]byte{})

	err := g.GenerateConfig(m, out)
	assert.Nil(t, err)
}
