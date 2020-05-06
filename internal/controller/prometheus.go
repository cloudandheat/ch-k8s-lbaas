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
package controller

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Collector struct {
	portmapper PortMapper

	servicesMetric *prometheus.GaugeVec
}

func NewCollector(portmapper PortMapper) *Collector {
	return &Collector{
		portmapper: portmapper,
		servicesMetric: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ch_k8s_lbaas_controller_services_total",
				Help: "Number of services by state",
			},
			[]string{"state"},
		),
	}
}

func (c *Collector) Describe(out chan<- *prometheus.Desc) {
	c.servicesMetric.Describe(out)
}

func (c *Collector) Collect(out chan<- prometheus.Metric) {
	model := c.portmapper.GetModel()
	c.servicesMetric.With(prometheus.Labels{"state": "mapped"}).Set(float64(len(model)))

	c.servicesMetric.Collect(out)
}
