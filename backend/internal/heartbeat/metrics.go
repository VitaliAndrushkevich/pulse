// Package heartbeat implements passive dead-man's-switch monitoring: a
// scheduled job pings Pulse on success, Pulse persists the next expected
// deadline, and a periodic-sweep evaluator raises an alert when a deadline
// passes with no ping.
package heartbeat

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the Prometheus collectors for the heartbeat subsystem. It is
// constructed with the shared registry in main.go and shared by the evaluator
// and the ping handler.
//
// No token material ever appears in a label — labels are limited to the
// heartbeat UUID and operator-chosen name, mirroring the pulse_monitor_up
// label style minus the type/url dimensions (not applicable to heartbeats).
type Metrics struct {
	// HeartbeatUp is 1 when the heartbeat's observed state is up, 0 when down.
	HeartbeatUp *prometheus.GaugeVec // labels: heartbeat_id, heartbeat_name
	// HeartbeatsTotal is the number of configured heartbeats, refreshed each tick.
	HeartbeatsTotal prometheus.Gauge
}

// NewMetrics creates and registers the heartbeat Prometheus metrics on the
// provided registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		HeartbeatUp: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pulse_heartbeat_up",
				Help: "Whether the heartbeat is up (1) or down (0).",
			},
			[]string{"heartbeat_id", "heartbeat_name"},
		),
		HeartbeatsTotal: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pulse_heartbeats_total",
				Help: "Total number of configured heartbeats.",
			},
		),
	}

	reg.MustRegister(m.HeartbeatUp)
	reg.MustRegister(m.HeartbeatsTotal)

	return m
}

// SetUp records the up/down gauge for a single heartbeat. Pass up=false on a
// missed-run or explicit-fail transition and up=true on success/recovery.
func (m *Metrics) SetUp(heartbeatID, heartbeatName string, up bool) {
	if m == nil {
		return
	}
	var v float64
	if up {
		v = 1.0
	}
	m.HeartbeatUp.WithLabelValues(heartbeatID, heartbeatName).Set(v)
}

// SetTotal records the total configured-heartbeat gauge.
func (m *Metrics) SetTotal(n int) {
	if m == nil {
		return
	}
	m.HeartbeatsTotal.Set(float64(n))
}
