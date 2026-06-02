package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds Prometheus metric collectors for the Pulse application.
type Metrics struct {
	MonitorUp           *prometheus.GaugeVec
	MonitorResponseTime *prometheus.GaugeVec
	MonitorsTotal       prometheus.Gauge
}

// NewMetrics creates and registers Prometheus metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		MonitorUp: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pulse_monitor_up",
				Help: "Whether the monitor is up (1) or down (0).",
			},
			[]string{"monitor_id", "monitor_name", "monitor_type"},
		),
		MonitorResponseTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pulse_monitor_response_time_seconds",
				Help: "Last recorded response time in seconds.",
			},
			[]string{"monitor_id", "monitor_name", "monitor_type"},
		),
		MonitorsTotal: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pulse_monitors_total",
				Help: "Total number of configured monitors.",
			},
		),
	}

	reg.MustRegister(m.MonitorUp)
	reg.MustRegister(m.MonitorResponseTime)
	reg.MustRegister(m.MonitorsTotal)

	return m
}

// RegisterMetricsRoute adds the /metrics endpoint to the router using the
// given Prometheus gatherer.
func RegisterMetricsRoute(r *gin.Engine, gatherer prometheus.Gatherer) {
	handler := promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
	r.GET("/metrics", gin.WrapH(handler))
}
