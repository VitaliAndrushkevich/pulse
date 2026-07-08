package handlers

import (
	"crypto/subtle"
	"net/http"

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
			[]string{"monitor_id", "monitor_name", "monitor_type", "monitor_url"},
		),
		MonitorResponseTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pulse_monitor_response_time_seconds",
				Help: "Last recorded response time in seconds.",
			},
			[]string{"monitor_id", "monitor_name", "monitor_type", "monitor_url"},
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
// given Prometheus gatherer. When user and password are both non-empty,
// the endpoint is protected by HTTP Basic Auth (constant-time comparison).
// When either is empty, the endpoint remains open for unauthenticated scraping.
func RegisterMetricsRoute(r *gin.Engine, gatherer prometheus.Gatherer, user, password string) {
	handler := promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})

	if user != "" && password != "" {
		r.GET("/metrics", metricsBasicAuth(user, password), gin.WrapH(handler))
	} else {
		r.GET("/metrics", gin.WrapH(handler))
	}
}

// metricsBasicAuth returns a gin middleware that validates HTTP Basic Auth
// credentials using constant-time comparison to prevent timing attacks.
func metricsBasicAuth(expectedUser, expectedPassword string) gin.HandlerFunc {
	expectedUserBytes := []byte(expectedUser)
	expectedPassBytes := []byte(expectedPassword)

	return func(c *gin.Context) {
		user, pass, hasAuth := c.Request.BasicAuth()
		if !hasAuth {
			c.Header("WWW-Authenticate", `Basic realm="metrics"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		userMatch := subtle.ConstantTimeCompare([]byte(user), expectedUserBytes) == 1
		passMatch := subtle.ConstantTimeCompare([]byte(pass), expectedPassBytes) == 1

		if !userMatch || !passMatch {
			c.Header("WWW-Authenticate", `Basic realm="metrics"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}
