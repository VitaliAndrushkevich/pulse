package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
)

const (
	// DefaultWorkers is the default worker pool size for production.
	DefaultWorkers = 200
	// DefaultDevWorkers is a reduced pool size suitable for local development.
	DefaultDevWorkers = 50
	// DefaultTickInterval is how often the scheduler polls for due monitors.
	DefaultTickInterval = 1 * time.Second
	// DefaultBatchSize is the maximum number of monitors fetched per tick.
	DefaultBatchSize int32 = 500
)

// SchedulerConfig holds tunable scheduler parameters.
type SchedulerConfig struct {
	Workers      int
	TickInterval time.Duration
	BatchSize    int32
}

// Scheduler orchestrates periodic monitor checks using a bounded worker pool.
type Scheduler struct {
	cfg           SchedulerConfig
	registry      *Registry
	queries       *db.Queries
	tsStore       *timescale.Store
	dynMetrics    *DynamicMetrics
	hub           *hub.Hub      // WebSocket broadcast hub (may be nil)
	encryptionKey []byte        // AES-256-GCM key for credential decryption
	wakeupCh      chan struct{} // signals immediate re-poll (from LISTEN/NOTIFY)
	stopOnce      sync.Once
}

// NewScheduler creates a scheduler with the given configuration and dependencies.
func NewScheduler(cfg SchedulerConfig, registry *Registry, queries *db.Queries, tsStore *timescale.Store, dynMetrics *DynamicMetrics, wsHub *hub.Hub, encryptionKey []byte) *Scheduler {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = DefaultTickInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	return &Scheduler{
		cfg:           cfg,
		registry:      registry,
		queries:       queries,
		tsStore:       tsStore,
		dynMetrics:    dynMetrics,
		hub:           wsHub,
		encryptionKey: encryptionKey,
		wakeupCh:      make(chan struct{}, 1),
	}
}

// Wakeup sends a non-blocking signal to the scheduler to re-poll immediately.
// Used by the LISTEN/NOTIFY listener when a monitor is created or updated.
func (s *Scheduler) Wakeup() {
	select {
	case s.wakeupCh <- struct{}{}:
	default:
		// Already signaled, skip.
	}
}

// Run starts the scheduling loop. It blocks until ctx is cancelled.
// Workers are bounded — no unbounded goroutine growth regardless of monitor count.
func (s *Scheduler) Run(ctx context.Context) {
	log.Printf("scheduler: starting with %d workers, tick=%s, batch=%d",
		s.cfg.Workers, s.cfg.TickInterval, s.cfg.BatchSize)

	// Buffered job channel provides backpressure.
	jobs := make(chan monitorJob, s.cfg.BatchSize)

	// Start bounded worker pool.
	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(ctx, jobs)
		}()
	}

	ticker := time.NewTicker(s.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("scheduler: shutting down")
			close(jobs)
			wg.Wait()
			log.Printf("scheduler: all workers stopped")
			return
		case <-ticker.C:
			s.poll(ctx, jobs)
		case <-s.wakeupCh:
			s.poll(ctx, jobs)
		}
	}
}

// monitorJob carries a monitor and its pre-loaded tags to a worker goroutine.
type monitorJob struct {
	monitor db.Monitor
	tags    map[string]string // key → value for metric labels
}

// tagEntry is used to unmarshal the JSON array returned by ListActiveMonitorsDueWithTags.
type tagEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// parseTagsJSON converts the tags_json interface{} from the query into a map.
func parseTagsJSON(raw interface{}) map[string]string {
	if raw == nil {
		return nil
	}

	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		// Try JSON marshaling as fallback for unexpected types.
		var err error
		data, err = json.Marshal(v)
		if err != nil {
			return nil
		}
	}

	var entries []tagEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	if len(entries) == 0 {
		return nil
	}

	result := make(map[string]string, len(entries))
	for _, e := range entries {
		result[e.Key] = e.Value
	}
	return result
}

// poll fetches due monitors with tags and dispatches them to the worker pool.
// It also triggers a RebuildLabels call when the set of tag keys changes.
func (s *Scheduler) poll(ctx context.Context, jobs chan<- monitorJob) {
	// Rebuild labels on every poll tick. This ensures new tag keys from
	// monitor create/update (triggered via LISTEN/NOTIFY wakeup) are picked
	// up even if no monitors are currently due. RebuildLabels is a no-op
	// when labels are unchanged.
	if s.dynMetrics != nil {
		allKeys, err := s.queries.ListAllTagKeys(ctx)
		if err != nil {
			log.Printf("scheduler: list tag keys for rebuild: %v", err)
		} else {
			s.dynMetrics.RebuildLabels(allKeys)
		}

		// Update total monitors gauge.
		count, err := s.queries.CountMonitors(ctx)
		if err == nil {
			s.dynMetrics.MonitorsTotal.Set(float64(count))
		}
	}

	rows, err := s.queries.ListActiveMonitorsDueWithTags(ctx, s.cfg.BatchSize)
	if err != nil {
		log.Printf("scheduler: poll error: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	// Build monitor jobs with pre-loaded tags.
	var monitorJobs []monitorJob

	for _, row := range rows {
		tags := parseTagsJSON(row.TagsJson)
		monitorJobs = append(monitorJobs, monitorJob{
			monitor: db.Monitor{
				ID:              row.ID,
				Name:            row.Name,
				Type:            row.Type,
				Target:          row.Target,
				IntervalSeconds: row.IntervalSeconds,
				TimeoutSeconds:  row.TimeoutSeconds,
				Status:          row.Status,
				State:           row.State,
				LastCheckedAt:   row.LastCheckedAt,
				NextCheckAt:     row.NextCheckAt,
				Settings:        row.Settings,
				CreatedAt:       row.CreatedAt,
				UpdatedAt:       row.UpdatedAt,
			},
			tags: tags,
		})
	}

	for _, job := range monitorJobs {
		select {
		case <-ctx.Done():
			return
		case jobs <- job:
		}
	}
}

// worker processes monitors from the jobs channel.
func (s *Scheduler) worker(ctx context.Context, jobs <-chan monitorJob) {
	for job := range jobs {
		if ctx.Err() != nil {
			return
		}
		s.executeCheck(ctx, job.monitor, job.tags)
	}
}

// executeCheck runs the appropriate checker for a monitor and persists results.
func (s *Scheduler) executeCheck(ctx context.Context, m db.Monitor, tags map[string]string) {
	checker, err := s.registry.Get(m.Type)
	if err != nil {
		log.Printf("scheduler: monitor %s: %v", m.ID, err)
		s.recordFailure(ctx, m, "unknown monitor type: "+m.Type)
		return
	}

	// Create a per-check context with the monitor's timeout.
	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// For gRPC monitors, inject the monitor_id into settings so the checker
	// can look up proto sources when payload_format is "proto_json".
	checkSettings := m.Settings
	if m.Type == "grpc" {
		checkSettings = injectMonitorID(m.Settings, m.ID)
	}

	// Load and decrypt credentials for HTTP and WebSocket monitors.
	var creds []AuthCredential
	if m.Type == "http" || m.Type == "http3" || m.Type == "websocket" {
		dbCreds, err := s.queries.ListCredentialsByMonitorIDInternal(ctx, m.ID)
		if err != nil {
			log.Printf("scheduler: monitor %s: load credentials: %v", m.ID, err)
		}
		for _, dc := range dbCreds {
			payload, err := decryptCredentialPayloadLocal(s.encryptionKey, dc.EncryptedValue)
			if err != nil {
				errMsg := fmt.Sprintf("credential decryption failure for monitor %s", m.ID)
				log.Printf("scheduler: %s", errMsg)
				s.recordFailure(ctx, m, errMsg)
				return
			}
			creds = append(creds, AuthCredential{
				AuthType:    dc.AuthType,
				Token:       payload.Token,
				Username:    payload.Username,
				Password:    payload.Password,
				HeaderName:  payload.HeaderName,
				HeaderValue: payload.HeaderValue,
			})
		}
	}

	// Use AuthenticatedChecker if credentials exist; otherwise fall back to Check.
	var result Result
	if len(creds) > 0 {
		if ac, ok := checker.(AuthenticatedChecker); ok {
			result = ac.CheckWithAuth(checkCtx, m.Target, checkSettings, creds)
		} else {
			result = checker.Check(checkCtx, m.Target, checkSettings)
		}
	} else {
		result = checker.Check(checkCtx, m.Target, checkSettings)
	}
	result.MonitorID = m.ID

	// Persist check result to TimescaleDB.
	if err := s.tsStore.WriteCheckResult(ctx, timescale.CheckPoint{
		MonitorID:        m.ID,
		State:            result.State,
		LatencyMs:        &result.LatencyMs,
		StatusCode:       result.StatusCode,
		Error:            strPtr(result.Error),
		SslDaysRemaining: result.SSLDaysRemaining,
		CheckedAt:        result.CheckedAt,
	}); err != nil {
		log.Printf("scheduler: monitor %s: write result: %v", m.ID, err)
	}

	// Also persist to the regular check_results table for the API layer.
	if _, err := s.queries.CreateCheckResult(ctx, db.CreateCheckResultParams{
		MonitorID:        m.ID,
		CheckedAt:        result.CheckedAt,
		State:            result.State,
		LatencyMs:        &result.LatencyMs,
		StatusCode:       result.StatusCode,
		Error:            strPtr(result.Error),
		SslDaysRemaining: result.SSLDaysRemaining,
	}); err != nil {
		log.Printf("scheduler: monitor %s: create check result: %v", m.ID, err)
	}

	// Update monitor state and schedule next check.
	nextCheck := time.Now().Add(time.Duration(m.IntervalSeconds) * time.Second)
	if _, err := s.queries.UpdateMonitorState(ctx, db.UpdateMonitorStateParams{
		ID:    m.ID,
		State: result.State,
		LastCheckedAt: pgtype.Timestamptz{
			Time:  result.CheckedAt,
			Valid: true,
		},
		NextCheckAt: pgtype.Timestamptz{
			Time:  nextCheck,
			Valid: true,
		},
	}); err != nil {
		log.Printf("scheduler: monitor %s: update state: %v", m.ID, err)
	}

	// Update Prometheus metrics via DynamicMetrics (tag-aware labels).
	if s.dynMetrics != nil {
		up := result.State == "up"
		s.dynMetrics.RecordCheck(
			m.ID.String(),
			m.Name,
			m.Type,
			metricMonitorURLLabel(m.Target),
			tags,
			up,
			float64(result.LatencyMs)/1000.0,
		)
	}

	// Broadcast status update via WebSocket hub (TASK-029).
	if s.hub != nil {
		s.hub.Broadcast(hub.NewMonitorStatusMessage(
			m.ID.String(),
			result.State,
			result.LatencyMs,
			result.StatusCode,
			result.SSLDaysRemaining,
			result.Error,
			result.CheckedAt,
		))
	}
}

// recordFailure handles cases where the checker itself couldn't be resolved.
func (s *Scheduler) recordFailure(ctx context.Context, m db.Monitor, errMsg string) {
	now := time.Now().UTC()
	nextCheck := now.Add(time.Duration(m.IntervalSeconds) * time.Second)

	var latency int32
	if _, err := s.queries.CreateCheckResult(ctx, db.CreateCheckResultParams{
		MonitorID:  m.ID,
		CheckedAt:  now,
		State:      "down",
		LatencyMs:  &latency,
		StatusCode: nil,
		Error:      &errMsg,
	}); err != nil {
		log.Printf("scheduler: monitor %s: record failure: %v", m.ID, err)
	}

	if _, err := s.queries.UpdateMonitorState(ctx, db.UpdateMonitorStateParams{
		ID:    m.ID,
		State: "down",
		LastCheckedAt: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		NextCheckAt: pgtype.Timestamptz{
			Time:  nextCheck,
			Valid: true,
		},
	}); err != nil {
		log.Printf("scheduler: monitor %s: update state after failure: %v", m.ID, err)
	}
}

// strPtr returns nil for empty strings, otherwise a pointer to the string.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// injectMonitorID merges monitor_id into a settings JSON blob so the checker
// can use it for DB lookups (e.g., proto source for proto_json payload format).
func injectMonitorID(settings json.RawMessage, id uuid.UUID) json.RawMessage {
	if len(settings) == 0 || string(settings) == "null" {
		return json.RawMessage(fmt.Sprintf(`{"monitor_id":"%s"}`, id.String()))
	}

	// Parse existing settings, inject monitor_id, re-marshal.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(settings, &m); err != nil {
		// Fallback: return original settings unchanged.
		return settings
	}

	idJSON, _ := json.Marshal(id.String())
	m["monitor_id"] = idJSON

	result, err := json.Marshal(m)
	if err != nil {
		return settings
	}
	return result
}

// metricMonitorURLLabel sanitizes a monitor target for safe exposure as a metric label.
func metricMonitorURLLabel(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return target
	}

	if u.Scheme == "" && u.Host == "" {
		return target
	}

	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""

	return u.String()
}

// credentialPayload mirrors the handlers.CredentialPayload struct to avoid
// circular imports between monitor and handlers packages.
type credentialPayload struct {
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"header_name,omitempty"`
	HeaderValue string `json:"header_value,omitempty"`
}

// decryptCredentialPayloadLocal decodes base64, decrypts via AES-256-GCM, and
// unmarshals into a credentialPayload. This is a local copy of the decryption
// logic to avoid circular imports with the handlers package.
func decryptCredentialPayloadLocal(key []byte, encrypted string) (credentialPayload, error) {
	var payload credentialPayload

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return payload, fmt.Errorf("credential crypto: decode base64: %w", err)
	}

	plaintext, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		return payload, fmt.Errorf("credential crypto: decrypt: %w", err)
	}

	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return payload, fmt.Errorf("credential crypto: unmarshal payload: %w", err)
	}

	return payload, nil
}
