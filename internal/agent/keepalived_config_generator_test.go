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
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudandheat/ch-k8s-lbaas/internal/model"
)

func newKeepalivedGenerator() *KeepalivedConfigGenerator {
	return &KeepalivedConfigGenerator{
		Priority:     23,
		VRRPPassword: "password",
		VRIDBase:     10,
		Interface:    "ethfoo",
	}
}

func TestKeepalivedGenerateStructuredConfigFromEmptyLBModel(t *testing.T) {
	g := newKeepalivedGenerator()

	m := &model.LoadBalancer{
		Ingress: []model.IngressIP{},
	}

	scfg, err := g.GenerateStructuredConfig(m)
	assert.Nil(t, err)
	assert.NotNil(t, scfg)
	assert.Equal(t, 0, len(scfg.Instances))
}

func TestKeepalivedGenerateStructuredConfigFromNonEmptyLBModel(t *testing.T) {
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

	i := scfg.Instances[0]

	assert.Equal(t, g.VRIDBase, i.VRID)
	assert.Equal(t, g.Priority, i.Priority)
	assert.Equal(t, g.Interface, i.Interface)
	assert.Equal(t, g.VRRPPassword, i.Password)
	assert.Equal(t, "VIPs", i.Name)
	assert.Equal(t, []keepalivedVRRPAddress{
		{Address: "127.0.0.1", Device: g.Interface},
		{Address: "127.0.0.2", Device: g.Interface},
		{Address: "127.0.0.3", Device: g.Interface},
	}, i.Addresses)
}

func TestKeepalivedGenerateStructuredConfigSortsByAddress(t *testing.T) {
	g := newKeepalivedGenerator()

	m := &model.LoadBalancer{
		Ingress: []model.IngressIP{
			{
				Address: "127.0.0.3",
			},
			{
				Address: "127.0.0.2",
			},
			{
				Address: "127.0.0.1",
			},
		},
	}

	scfg, err := g.GenerateStructuredConfig(m)
	assert.Nil(t, err)
	assert.NotNil(t, scfg)
	assert.Equal(t, 1, len(scfg.Instances))

	i := scfg.Instances[0]

	assert.Equal(t, g.VRIDBase, i.VRID)
	assert.Equal(t, g.Priority, i.Priority)
	assert.Equal(t, g.Interface, i.Interface)
	assert.Equal(t, g.VRRPPassword, i.Password)
	assert.Equal(t, "VIPs", i.Name)
	assert.Equal(t, []keepalivedVRRPAddress{
		{Address: "127.0.0.1", Device: g.Interface},
		{Address: "127.0.0.2", Device: g.Interface},
		{Address: "127.0.0.3", Device: g.Interface},
	}, i.Addresses)
}

func TestKeepalivedGenerateConfigFromNonEmptyLBModel(t *testing.T) {
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
