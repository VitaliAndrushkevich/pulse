package influx

import (
	"context"
	"fmt"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

const measurement = "monitor_checks"

// CheckPoint is a single monitor check result written to or read from InfluxDB.
//
// Schema conventions:
//
//	Measurement: monitor_checks
//	Tags:        monitor_id   - UUID of the monitor (string)
//	             monitor_type - protocol type: http, https, tcp, udp, websocket
//	             state        - check outcome: up, down, degraded
//	Fields:      latency_ms   - round-trip latency in milliseconds (float64)
//	             status_code  - HTTP status code; 0 when not applicable (int64)
//	Timestamp:   time the check was performed (nanosecond precision)
type CheckPoint struct {
	MonitorID   string
	MonitorType string
	State       string
	LatencyMs   float64
	StatusCode  int64
	CheckedAt   time.Time
}

// Store wraps an InfluxDB v2 client and exposes write/query helpers.
type Store struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	queryAPI api.QueryAPI
	org      string
	bucket   string
}

// New creates a Store connected to the given InfluxDB endpoint.
// url    - e.g. "http://localhost:8086"
// token  - InfluxDB API token
// org    - organisation name or ID
// bucket - target bucket name
func New(url, token, org, bucket string) *Store {
	client := influxdb2.NewClient(url, token)
	return &Store{
		client:   client,
		writeAPI: client.WriteAPIBlocking(org, bucket),
		queryAPI: client.QueryAPI(org),
		org:      org,
		bucket:   bucket,
	}
}

// NewFromEnv constructs a Store using environment variables with sane
// defaults for a self-hosted Docker Compose setup.
//
// Environment variables:
// - INFLUXDB_URL (default: "http://influxdb:8086")
// - INFLUXDB_TOKEN (default: "")
// - INFLUXDB_ORG  (default: "pulse")
// - INFLUXDB_BUCKET (default: "pulse")
func NewFromEnv() *Store {
	url := getenv("INFLUXDB_URL", "http://influxdb:8086")
	token := getenv("INFLUXDB_TOKEN", "")
	org := getenv("INFLUXDB_ORG", "pulse")
	bucket := getenv("INFLUXDB_BUCKET", "pulse")
	return New(url, token, org, bucket)
}

func getenv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// Close releases the underlying InfluxDB client resources.
func (s *Store) Close() {
	s.client.Close()
}

// Ping verifies connectivity to InfluxDB.
func (s *Store) Ping(ctx context.Context) error {
	ok, err := s.client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("influx ping: %w", err)
	}
	if !ok {
		return fmt.Errorf("influx ping: server not ready")
	}
	return nil
}

// WriteCheckResult writes a single monitor check result to InfluxDB.
func (s *Store) WriteCheckResult(ctx context.Context, pt CheckPoint) error {
	p := write.NewPoint(
		measurement,
		map[string]string{
			"monitor_id":   pt.MonitorID,
			"monitor_type": pt.MonitorType,
			"state":        pt.State,
		},
		map[string]interface{}{
			"latency_ms":  pt.LatencyMs,
			"status_code": pt.StatusCode,
		},
		pt.CheckedAt,
	)
	if err := s.writeAPI.WritePoint(ctx, p); err != nil {
		return fmt.Errorf("influx write: %w", err)
	}
	return nil
}

// QueryHistory returns check results for a monitor within [start, end).
// Results are ordered by time ascending.
func (s *Store) QueryHistory(ctx context.Context, monitorID string, start, end time.Time) ([]CheckPoint, error) {
	flux := fmt.Sprintf(
		`from(bucket: %q)
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == %q and r.monitor_id == %q)
  |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
  |> sort(columns: ["_time"], desc: false)`,
		s.bucket,
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
		measurement,
		monitorID,
	)

	result, err := s.queryAPI.Query(ctx, flux)
	if err != nil {
		return nil, fmt.Errorf("influx query: %w", err)
	}
	defer result.Close()

	var points []CheckPoint
	for result.Next() {
		rec := result.Record()
		pt := CheckPoint{
			MonitorID: monitorID,
			CheckedAt: rec.Time(),
		}
		if v, ok := rec.ValueByKey("monitor_type").(string); ok {
			pt.MonitorType = v
		}
		if v, ok := rec.ValueByKey("state").(string); ok {
			pt.State = v
		}
		if v, ok := rec.ValueByKey("latency_ms").(float64); ok {
			pt.LatencyMs = v
		}
		if v, ok := rec.ValueByKey("status_code").(int64); ok {
			pt.StatusCode = v
		}
		points = append(points, pt)
	}
	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("influx query result: %w", err)
	}
	return points, nil
}
