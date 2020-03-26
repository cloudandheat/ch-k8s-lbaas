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
