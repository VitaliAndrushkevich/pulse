# Performance Findings — Backend Scheduler and Concurrency

## PERF-020: Database Connection Pool Uses pgx Defaults — Undersized for Worker Count

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Database Pool Sizing |
| **Effort** | Small (hours) |
| **Priority** | 5 |

**Description:** The database connection pool is created via `pgxpool.New(ctx, databaseURL)` without any explicit pool configuration. The pgx/v5 default `MaxConns` is `max(4, runtime.NumCPUs())` — typically 4–16 on most deployments. In production, the scheduler runs 200 workers and the notification dispatcher runs 50 workers, all competing for connections from this single shared pool. Under sustained load with 500 monitors, connection exhaustion is almost certain: scheduler workers will block on pool acquisition, increasing check latency and potentially causing cascading timeouts.

**Evidence:**
`backend/internal/store/postgres/pool.go:14-22`
```go
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres pool init: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return pool, nil
}
```

**Impact:** Under load, scheduler workers and notification workers will contend for a small connection pool. Workers block on `pool.Acquire()`, causing check-cycle durations to spike. At 500 monitors with 1-second tick intervals, this becomes a bottleneck that prevents timely monitoring and degrades notification delivery latency.

**Remediation:** Parse the `DATABASE_URL` with `pgxpool.ParseConfig`, then set `MaxConns` to at least `PULSE_SCHEDULER_WORKERS + PULSE_NOTIFICATION_WORKERS + 20` (270 in default prod config). Also set `MinConns` to a warm baseline (e.g. 20) and `MaxConnLifetime` / `MaxConnIdleTime` for connection health. The property restored is: no connection starvation under sustained 500-monitor load.

---

## PERF-021: Scheduler N+1 Query Pattern in Notification Fan-Out Path

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | N+1 Query Pattern |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** For every monitor check that has notification bindings, the scheduler executes three sequential queries per monitor in `dispatchNotifications`: (1) `ListBindingsByMonitor` to fetch bindings, (2) `ListCheckResultsByMonitor` with LIMIT 100 to count consecutive failures, and (3) the fan-out enqueues jobs. Then, each delivery worker executes up to 3 more queries per job in `dispatch()`: `GetChannel`, `GetMonitor` (via `enrichPayload`), and `GetOpenIncidentByMonitor`. With 500 monitors and multiple bindings each, this pattern produces thousands of small queries per tick cycle.

**Evidence:**
`backend/internal/monitor/scheduler.go:237-262`
```go
func (s *Scheduler) dispatchNotifications(ctx context.Context, m db.Monitor, result Result) {
	dbBindings, err := s.queries.ListBindingsByMonitor(ctx, m.ID)
	// ...
	consecFailures := s.countConsecutiveFailures(ctx, m.ID)
	// ...
}
```

`backend/internal/notification/dispatcher.go:196-202`
```go
func (d *Dispatcher) dispatch(job DeliveryJob) error {
	// ...
	channel, err := d.queries.GetChannel(ctx, job.ChannelID)
	// ...
	d.enrichPayload(ctx, &job)
	// ...
}
```

**Impact:** At 500 monitors with bindings, each tick can generate 1000+ DB round-trips for notification evaluation alone. Combined with the undersized pool (PERF-020), this amplifies connection contention. The `countConsecutiveFailures` call fetches 100 rows when only the first few "down" states are needed — wasteful I/O.

**Remediation:** (1) Batch-load bindings for all due monitors in a single query during `poll()` rather than per-monitor. (2) Replace `ListCheckResultsByMonitor(LIMIT 100)` with a dedicated `CountConsecutiveFailures` query that uses a window function or early termination. (3) Cache channel configs in the dispatcher (channels rarely change) to eliminate per-job `GetChannel` lookups. The property restored is: O(1) DB queries per tick regardless of monitor count.

---

## PERF-022: No TimescaleDB Native Retention Policy — Custom DELETE with Subquery

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | TimescaleDB Configuration |
| **Effort** | Small (hours) |
| **Priority** | 10 |

**Description:** The TimescaleDB hypertable `check_results` has no native `add_retention_policy`. Instead, a custom Go `RetentionService` iterates monitors in batches and runs `DELETE FROM check_results WHERE id IN (SELECT id ... WHERE monitor_id = $1 AND checked_at < $2 LIMIT $3)` per monitor. This approach is significantly slower than TimescaleDB's native `drop_chunks()` because it performs row-level deletion rather than chunk-level drops. For per-monitor variable retention, this is a necessary trade-off, but it creates sustained I/O pressure during cleanup cycles and does not leverage TimescaleDB compression or continuous aggregates.

**Evidence:**
`backend/internal/retention/service.go:155-167`
```go
func (s *RetentionService) deleteExpiredRows(ctx context.Context, mon db.Monitor) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(mon.HistoryRetentionDays) * 24 * time.Hour)

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM check_results
		 WHERE id IN (
		   SELECT id FROM check_results
		   WHERE monitor_id = $1
		     AND checked_at < $2
		   LIMIT $3
		 )`,
		mon.ID, cutoff, s.deleteLimit,
	)
	// ...
}
```

`backend/migrations/004_timescaledb_check_results.up.sql:4-5`
```sql
CREATE EXTENSION IF NOT EXISTS timescaledb;
SELECT create_hypertable('check_results', 'checked_at', if_not_exists => TRUE);
```

**Impact:** With 500 monitors at 1-second check intervals and 30-day retention, the table accumulates ~1.3 billion rows. Row-level DELETE with a subquery is orders of magnitude slower than chunk-based `drop_chunks()`. Cleanup cycles will consume significant DB resources and hold row locks, potentially interfering with concurrent inserts from the scheduler.

**Remediation:** Add a TimescaleDB `add_retention_policy('check_results', INTERVAL '365 days')` as a safety net for the maximum retention window. Enable chunk-based compression (`add_compression_policy`) for data older than a configurable threshold (e.g. 7 days). Keep the per-monitor custom retention for variable windows, but have it run less frequently and in smaller batches during off-peak. The property restored is: time-range queries over the maximum retention window complete efficiently with chunk-level operations.

---

## PERF-023: Hub ClientCount() Uses Mutex Not Updated by Run Loop

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Concurrency Correctness |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** `Hub.ClientCount()` reads `len(h.clients)` under `h.mu.RLock()`, but the `Run()` goroutine modifies `h.clients` (register/unregister/drop) without holding this mutex — it uses a single-goroutine ownership model via channels. This means `ClientCount()` can observe stale or torn reads if the map is being modified concurrently. In practice, Go maps are not concurrency-safe and reading `len()` while another goroutine is writing (register/unregister) is a data race.

**Evidence:**
`backend/internal/hub/hub.go:126-131`
```go
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
```

`backend/internal/hub/hub.go:69-94` (Run loop modifies `h.clients` without locking `h.mu`)
```go
case client := <-h.register:
	h.clients[client] = struct{}{}
case client := <-h.unregister:
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
	}
```

**Impact:** Potential data race detected by `-race` flag. In production, this is unlikely to cause a crash (the map length read is atomic on most platforms), but it violates Go's memory model and could produce incorrect metrics. The broadcast fan-out and slow-consumer eviction themselves are safe (single-goroutine ownership), so the core functionality is not affected.

**Remediation:** Either (a) use an `atomic.Int64` counter incremented/decremented in the `Run()` goroutine, or (b) expose `ClientCount()` as a channel-based request to the `Run()` loop. The property restored is: race-free client count reads under Go's memory model.

---

## PERF-024: Scheduler Poll Rebuilds Prometheus Labels on Every Tick

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Scheduler Efficiency |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The `poll()` method calls `queries.ListAllTagKeys(ctx)` and `queries.CountMonitors(ctx)` on every single tick (1-second interval) regardless of whether any monitors are due. `RebuildLabels` is documented as a no-op when labels are unchanged, but the two DB queries still execute every second. With 500+ monitors, this adds 2 unnecessary queries per second (86,400/day) that compete for DB connections.

**Evidence:**
`backend/internal/monitor/scheduler.go:141-157`
```go
func (s *Scheduler) poll(ctx context.Context, jobs chan<- monitorJob) {
	if s.dynMetrics != nil {
		allKeys, err := s.queries.ListAllTagKeys(ctx)
		if err != nil {
			log.Printf("scheduler: list tag keys for rebuild: %v", err)
		} else {
			s.dynMetrics.RebuildLabels(allKeys)
		}

		count, err := s.queries.CountMonitors(ctx)
		if err == nil {
			s.dynMetrics.MonitorsTotal.Set(float64(count))
		}
	}
	// ...
}
```

**Impact:** Two extra queries per second add negligible per-query cost, but consume connection pool slots under contention (see PERF-020). Under the undersized pool, these calls can delay actual check dispatching by blocking on pool acquisition.

**Remediation:** Debounce `ListAllTagKeys` and `CountMonitors` to run at most every 30–60 seconds (e.g., with a `time.Time` last-run check). Alternatively, move gauge updates to a separate low-frequency goroutine outside the hot path. The property restored is: the scheduler poll hot path executes only check-dispatch queries.

---

## PERF-025: Notification Dispatcher Worker Uses context.Background() for Delivery

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Context Propagation |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The notification dispatcher `worker()` calls `processJob(context.Background(), ...)`, and `dispatch()` creates its own `context.WithTimeout(context.Background(), 30*time.Second)`. When the application receives SIGTERM, the `Shutdown()` method signals workers via `close(d.done)`, but in-flight `dispatch()` calls have contexts that are independent of the shutdown signal. Workers will continue processing up to their 30-second timeout even after shutdown is initiated, relying only on the `wg.Wait()` with `DrainTimeout` as a hard stop.

**Evidence:**
`backend/internal/notification/dispatcher.go:169-179`
```go
func (d *Dispatcher) worker(id int) {
	defer d.wg.Done()
	for {
		select {
		case job, ok := <-d.jobs:
			if !ok {
				return
			}
			d.processJob(context.Background(), id, job)
		case <-d.done:
			return
		}
	}
}
```

`backend/internal/notification/dispatcher.go:196-198`
```go
func (d *Dispatcher) dispatch(job DeliveryJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
```

**Impact:** During graceful shutdown, notification deliveries that started just before the shutdown signal will have up to 30 seconds to complete (unaware of shutdown). The `DrainTimeout` (default 30s) provides a hard boundary, but individual deliveries cannot be cancelled cooperatively. This is mitigated by the drain timeout, making the practical risk low.

**Remediation:** Pass a shutdown-aware context (derived from the application context or a dedicated cancellation context) to `processJob` and `dispatch`. This allows in-flight deliveries to be cancelled promptly on shutdown rather than relying on the drain timeout. The property restored is: cooperative cancellation of in-flight work during graceful shutdown.

---

## PERF-026: Scheduler Job Channel Blocks Senders When Workers Are Busy

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Backpressure Design |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The scheduler's `poll()` method sends jobs to the buffered `jobs` channel (capacity = `BatchSize`, default 500) using a blocking `select` with `ctx.Done()` fallback. When all 200 workers are busy and the buffer is full, `poll()` blocks until a worker becomes available. This is correct behavior — it provides natural backpressure without dropping checks or panicking. The design ensures fair distribution: all due monitors are fetched per tick (up to BatchSize) and queued in order, with workers draining them FIFO.

**Evidence:**
`backend/internal/monitor/scheduler.go:185-190`
```go
for _, job := range monitorJobs {
	select {
	case <-ctx.Done():
		return
	case jobs <- job:
	}
}
```

**Impact:** None — this is correct design. Under extreme load (all 500 monitors due simultaneously with all workers occupied), the scheduler's polling goroutine will block until workers free up. The next tick will be skipped, but monitors will be processed within 2 tick intervals of their `next_check_at` due to the ordered fetch (`ORDER BY next_check_at ASC NULLS FIRST`).

**Remediation:** No action required. This is the intended bounded-concurrency pattern. For observability, consider adding a Prometheus histogram tracking time spent blocking on the jobs channel to detect when worker saturation occurs.

---

## PERF-027: WebSocket Hub Broadcast Fan-Out — Correct Slow-Consumer Eviction

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | WebSocket Throughput |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The hub broadcasts to all clients using a `select/default` pattern per client. If a client's send buffer (256 messages) is full, the client is immediately disconnected and removed from the client map within the same broadcast cycle — no blocking of delivery to other clients occurs. The hub's own broadcast channel is also buffered (256) with a non-blocking drop on overflow, preventing the scheduler from blocking on hub broadcasts. This meets the requirement for fan-out to 200+ clients with sub-100ms latency at p95.

**Evidence:**
`backend/internal/hub/hub.go:85-93`
```go
case message := <-h.broadcast:
	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			delete(h.clients, client)
			close(client.send)
			log.Printf("hub: client %s dropped (slow consumer)", client.ID)
		}
	}
```

**Impact:** None — this is correct design. Slow consumers are evicted within one broadcast cycle. The single-goroutine event loop ensures no lock contention during fan-out. Write coalescing in `writePump()` batches queued messages into a single syscall, reducing per-message overhead.

**Remediation:** No action required. For observability at scale (200+ clients), consider adding a Prometheus gauge for connected client count and a counter for slow-consumer evictions to monitor hub health.

---

## PERF-028: Notification Dispatcher — Correct Independence from Scheduler

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Concurrency Architecture |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The notification dispatcher operates a fully independent worker pool (`PULSE_NOTIFICATION_WORKERS`, default 50) on a separate buffered channel (capacity 256). The scheduler enqueues jobs via `Enqueue()` which uses `select/default` — never blocks the calling goroutine. When the buffer is full, jobs are dropped with a metric increment (`pulse_notification_dropped_total`) rather than blocking the scheduler's check cycle. This ensures notification delivery latency cannot increase scheduler check-cycle duration by more than the time to execute the `select/default` statement (~nanoseconds).

**Evidence:**
`backend/internal/notification/dispatcher.go:141-154`
```go
func (d *Dispatcher) Enqueue(job DeliveryJob) {
	if d.stopping.Load() != 0 {
		channelType := classifyChannelType(job)
		d.metrics.DroppedTotal.WithLabelValues(channelType).Inc()
		return
	}
	select {
	case d.jobs <- job:
	default:
		channelType := classifyChannelType(job)
		d.metrics.DroppedTotal.WithLabelValues(channelType).Inc()
		log.Printf("notification: job dropped (buffer full) monitor=%s trigger=%s", job.MonitorID, job.TriggerType)
	}
}
```

**Impact:** None — this is correct design. The scheduler and notification dispatcher are decoupled via a buffered channel with non-blocking enqueue. Worker pools do not share goroutines.

**Remediation:** No action required. The `pulse_notification_dropped_total` metric provides visibility into buffer saturation. Alert on this metric rising above zero to detect undersized notification worker pools.
