# Backend Code Quality Findings (QUAL-020 through QUAL-029)

## Finding Template Reference
Each finding follows the format: Finding ID, Severity, Category, Title, Description, Evidence, Impact, Remediation, Effort, Priority.

---

### QUAL-020: HTTP server lacks graceful shutdown — in-flight requests aborted on SIGTERM

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | shutdown |
| **Effort** | Small (hours) |
| **Priority** | 5 |

**Description:** The application uses `gin.Engine.Run()` which internally calls `http.ListenAndServe` — a blocking call with no shutdown hook. The shutdown goroutine cancels the `appCtx`, stops the hub, and drains the notification dispatcher, but never calls `http.Server.Shutdown(ctx)` on the HTTP listener. This means in-flight API requests are terminated immediately when the process receives SIGTERM, violating the expected drain sequence: stop accepting → drain in-flight → close connections → exit.

**Evidence:**
`backend/cmd/pulse/main.go:225-226`
```go
	if err := r.Run(addr); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
```

The shutdown goroutine (line 207–222) stops background services but has no reference to an `http.Server` to call `.Shutdown()`:
```go
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %s, shutting down...", sig)
		reminderScheduler.Stop()
		drainCtx, drainCancel := context.WithTimeout(context.Background(), notifDrainTimeout)
		dispatcher.Shutdown(drainCtx)
		drainCancel()
		wsHub.Stop()
		appCancel()
	}()
```

**Impact:** Any HTTP request in progress at shutdown is killed mid-flight. Clients receive connection-reset errors. Database transactions may be left uncommitted. Under rolling deploys, this causes brief request failures.

**Remediation:** Replace `r.Run(addr)` with an explicit `http.Server{Handler: r}` and call `srv.Shutdown(ctx)` with a drain timeout in the signal handler before cancelling background services. The correct sequence is: (1) `srv.Shutdown()` to stop accepting and drain HTTP, (2) stop background services, (3) close DB pool, (4) exit. This restores the graceful-drain property.

---

### QUAL-021: Notification dispatcher workers use context.Background() — unlinked from app lifecycle

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | context-propagation |
| **Effort** | Small (hours) |
| **Priority** | 10 |

**Description:** The dispatcher's `worker()` method calls `d.processJob(context.Background(), id, job)` for each job. This means delivery operations (DB lookups, HTTP webhook calls, SMTP sends) run with an unbounded context not tied to application shutdown. If the `done` channel fires while a job is in-progress, the context will never cancel — the delivery must complete or hit its own internal timeout (30s in `dispatch()`). This is partially mitigated by the 30s timeout inside `dispatch()`, but the parent context should derive from a cancellable source.

**Evidence:**
`backend/internal/notification/dispatcher.go:246-247`
```go
	case job, ok := <-d.jobs:
		if !ok {
			return
		}
		d.processJob(context.Background(), id, job)
```

**Impact:** During shutdown, workers may hang for up to 30 seconds per in-flight delivery even after the drain signal fires, because the context given to HTTP/SMTP calls won't cancel proactively. The WaitGroup timeout in `Shutdown()` handles the worst case, but it could drain faster if jobs respected a cancellation signal.

**Remediation:** Pass a context derived from the dispatcher's lifecycle (e.g., a context cancelled when `d.done` fires) instead of `context.Background()`. The `processJob` timeout of 30s would then be `min(30s, time-until-shutdown-deadline)`, allowing faster drain. This restores the context-propagation property.

---

### QUAL-022: ICMP resolver uses context.Background() — ignores per-check timeout

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | context-propagation |
| **Effort** | Small (hours) |
| **Priority** | 11 |

**Description:** The `resolveAddr()` helper in the ICMP checker calls `net.DefaultResolver.LookupIP(context.Background(), ...)`. This DNS lookup is not bounded by the per-check timeout context passed to `Check()`. A DNS lookup that hangs (e.g., against a misconfigured or unreachable resolver) will block the worker goroutine indefinitely, bypassing the monitor's configured `TimeoutSeconds`.

**Evidence:**
`backend/internal/monitor/icmp.go:235`
```go
	ips, err := net.DefaultResolver.LookupIP(context.Background(), network, addr)
```

**Impact:** A stalled DNS resolution can block a scheduler worker indefinitely. With a bounded worker pool, enough stalled ICMP monitors could exhaust all workers and halt the scheduler entirely.

**Remediation:** Accept the check context as a parameter: `func resolveAddr(ctx context.Context, addr string, useIPv6 bool)` and pass the per-check context to `LookupIP`. This ensures DNS resolution respects the monitor timeout. Restores the timeout-enforcement property.

---

### QUAL-023: Hub.ClientCount() has a data race — reads h.clients without synchronization with Run() loop

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | resource-leak |
| **Effort** | Small (hours) |
| **Priority** | 10 |

**Description:** `Hub.ClientCount()` reads `h.clients` (a map) protected by `h.mu.RLock()`, but the `Run()` event loop modifies `h.clients` (add/remove/delete) without ever acquiring `h.mu`. This is a classic data race: the reader takes the mutex but the writer never does. Under Go's memory model, concurrent unsynchronized read and write to a map is undefined behavior and will crash with `fatal error: concurrent map read and map write` under race detector.

**Evidence:**
`backend/internal/hub/hub.go:127-130`
```go
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
```

Versus `Run()` (lines 74–98) which mutates `h.clients` without locking:
```go
case client := <-h.register:
    h.clients[client] = struct{}{}
```

**Impact:** If `ClientCount()` is called concurrently with `Run()` (e.g., from a metrics endpoint or diagnostic handler), the program will crash or produce corrupted results. Currently `ClientCount()` is unused externally — but it's an exported method, inviting future use that would trigger the race.

**Remediation:** Either (a) remove the exported `ClientCount()` method since it's unused, or (b) wrap all `h.clients` mutations in `Run()` with `h.mu.Lock()`/`h.mu.Unlock()`. Option (a) is simpler; option (b) is correct if the method is needed. Restores the thread-safety property.

---

### QUAL-024: Notification worker's processJob uses log + metric but does not return error to caller — half-handling pattern

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | error-handling |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** In the dispatcher's `processJob()`, errors from `dispatch()` are logged, recorded in delivery_logs, and counted in Prometheus metrics — but the error is never propagated upward. This is intentional for fire-and-forget delivery, but it means the retry queue logic is embedded inside `dispatch()` rather than managed by the caller. The pattern is acceptable but not idiomatic: the worker consumes the error rather than letting a higher-level orchestrator decide on retry vs drop. This finding is informational — the behavior is correct, but the retry mechanism is implicit.

**Evidence:**
`backend/internal/notification/dispatcher.go:287-294`
```go
	if err != nil {
		d.metrics.DeliveriesTotal.WithLabelValues(channelType, "failure").Inc()
		log.Printf("notification: delivery failed ...")
		d.LogDelivery(ctx, job.ChannelID, job.MonitorID, job.BindingID,
			job.TriggerType, job.Attempt, "failure", err.Error())
	}
```

**Impact:** Low. The pattern works but makes retry logic less visible. Future maintainers may not realize that retry is handled externally by the retry queue rather than by the worker loop.

**Remediation:** Consider extracting the retry decision from inside `dispatch()`/delivery methods into `processJob()` — checking `IsRetryable(err)` at the worker level and explicitly re-enqueueing. This makes the control flow more transparent. Restores the single-responsibility property.

---

### QUAL-025: Timescale and retention stores use raw SQL via pool.Exec/Query — bypass sqlc layer

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | sqlc-consistency |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** The `internal/store/timescale/` package and `internal/retention/` service execute SQL directly via `pool.Exec()` and `pool.Query()` with parameterized queries, bypassing the sqlc-generated code layer. While these queries use proper parameterization (no SQL injection risk), they don't benefit from sqlc's compile-time type checking, automatic regeneration on schema changes, or centralized query management.

**Evidence:**
`backend/internal/store/timescale/store.go:60-71`
```go
	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO check_results (monitor_id, checked_at, state, latency_ms, status_code, error, ssl_days_remaining)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		pt.MonitorID, pt.CheckedAt, pt.State, pt.LatencyMs, pt.StatusCode, pt.Error, pt.SslDaysRemaining,
	)
```

`backend/internal/retention/service.go:204-210`
```go
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM check_results
		 WHERE id IN (
		   SELECT id FROM check_results
		   WHERE monitor_id = $1
		     AND checked_at < $2
		   LIMIT $3
		 )`,
```

**Impact:** Low — queries are parameterized so no injection risk. However, schema changes to `check_results` could silently break these queries without build-time detection. Also creates inconsistency: some queries are managed by sqlc, others are hand-written.

**Remediation:** Migrate the timescale store queries and retention DELETE into sqlc `.sql` files. For the `time_bucket()` and `LIMIT` subquery patterns that sqlc may struggle with, consider using `-- name: X :exec` with explicit SQL. The `QueryHistoryAggregated` function with dynamic interval formatting may need `sqlc.arg()` placeholders. This restores the single-query-management-layer property.

---

### QUAL-026: Proto registry uses context.Background() for compilation — no cancellation propagation

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | context-propagation |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The proto registry's `ParseProtoFiles()` method calls `compiler.Compile(context.Background(), ...)`. This compilation step processes user-uploaded `.proto` files and could take arbitrary time for large file sets. The background context means no timeout or cancellation from the HTTP request handler propagates — a maliciously large proto file set could hang the handler goroutine.

**Evidence:**
`backend/internal/proto/registry.go:78`
```go
	compiled, err := compiler.Compile(context.Background(), filenames...)
```

**Impact:** A user uploading a large or pathological proto file set could cause the handler to hang. Since this is behind authentication and the API is admin-only, the blast radius is limited to self-inflicted DoS.

**Remediation:** Accept a context parameter from the caller (the HTTP handler's `c.Request.Context()`) and pass it to `compiler.Compile()`. This ensures the compilation respects the request timeout and client disconnection. Restores the timeout-propagation property.

---

### QUAL-027: Missing error wrapping in reflect.go — fmt.Errorf without %w breaks error chain

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | error-handling |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** Several error returns in `internal/proto/reflect.go` use `fmt.Errorf("...: %s", err)` or `fmt.Errorf("...")` without the `%w` verb, breaking `errors.Is()` and `errors.As()` chains. Callers cannot programmatically distinguish between connection failures, timeouts, and unsupported-reflection errors without string matching.

**Evidence:**
`backend/internal/proto/reflect.go:80-81`
```go
	if errResp := resp.GetErrorResponse(); errResp != nil {
		return nil, fmt.Errorf("server does not support reflection: %s", errResp.GetErrorMessage())
	}
```

`backend/internal/proto/reflect.go:120-121`
```go
		if errResp := resp.GetErrorResponse(); errResp != nil {
			return nil, fmt.Errorf("failed to get descriptor for service %q: %s", svcName, errResp.GetErrorMessage())
		}
```

**Impact:** Callers cannot use `errors.Is(err, ErrReflectionUnsupported)` or similar sentinel checks to handle different gRPC reflection failures. Currently the handler catches all errors generically, but future error-specific retry logic would require string parsing.

**Remediation:** Define sentinel errors (`var ErrReflectionUnsupported = errors.New(...)`) and wrap underlying errors with `%w`. For string-sourced errors from gRPC responses, wrap them as: `fmt.Errorf("reflection: %s: %w", msg, ErrReflectionUnsupported)`. Restores the error-chain property.

---

### QUAL-028: Hub.Run() goroutine is not context-aware — relies solely on channel close for termination

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | resource-leak |
| **Effort** | Small (hours) |
| **Priority** | 15 |

**Description:** The `Hub.Run()` goroutine uses `h.done` channel close as its termination signal. While functional, it doesn't accept or select on a `context.Context` — meaning it cannot participate in the application's context cancellation hierarchy. The shutdown sequence in `main.go` calls `wsHub.Stop()` which closes `h.done`, but this is an independent mechanism from the `appCtx` cancellation used by the scheduler and retention service.

**Evidence:**
`backend/internal/hub/hub.go:68-69`
```go
func (h *Hub) Run() {
	log.Printf("hub: started")
	for {
		select {
		...
		case <-h.done:
			// Shutdown: close all client connections.
```

`backend/cmd/pulse/main.go:160`
```go
	go wsHub.Run()
```

**Impact:** Low — the current shutdown sequence works correctly because `Stop()` is called explicitly before `appCancel()`. However, if the shutdown ordering changes or a bug skips `Stop()`, the hub goroutine would leak. This is defense-in-depth.

**Remediation:** Modify `Run()` to accept a `context.Context` and select on `ctx.Done()` in addition to `h.done`. This provides a fallback termination path aligned with the application lifecycle. Restores the context-hierarchy property.

---

### QUAL-029: Shutdown sequence does not drain HTTP server before stopping background services

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | shutdown |
| **Effort** | Small (hours) |
| **Priority** | 9 |

**Description:** The required graceful shutdown sequence is: (1) stop accepting new requests, (2) drain in-flight requests, (3) close persistent connections, (4) exit. The current implementation reverses steps 1-2 and 3: it stops the notification dispatcher and WebSocket hub first, then cancels the app context. The HTTP server (`r.Run`) never stops accepting — it continues serving requests while background services are being torn down. A request arriving during shutdown could attempt to use already-stopped services (hub, dispatcher) and fail with nil-pointer or closed-channel errors.

**Evidence:**
`backend/cmd/pulse/main.go:207-222`
```go
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %s, shutting down...", sig)

		// Stop reminder scheduler first (no new reminders enqueued).
		reminderScheduler.Stop()

		// Drain the notification dispatcher within its configured timeout.
		drainCtx, drainCancel := context.WithTimeout(context.Background(), notifDrainTimeout)
		dispatcher.Shutdown(drainCtx)
		drainCancel()

		wsHub.Stop()
		appCancel()
	}()
```

**Impact:** Requests arriving after the hub/dispatcher stop but before the process exits will encounter partially-torn-down services. WebSocket clients will receive connection resets rather than graceful close frames. Notifications triggered by final check results may be silently dropped.

**Remediation:** Correct the sequence to: (1) `srv.Shutdown(drainCtx)` — stops accepting, drains in-flight HTTP, (2) `appCancel()` — stops scheduler and retention, (3) drain notification dispatcher, (4) `wsHub.Stop()`, (5) `pool.Close()`. This ensures no new requests arrive while services are shutting down. Restores the graceful-drain-ordering property.

---

## Summary

| ID | Severity | Category | Title |
|----|----------|----------|-------|
| QUAL-020 | High | shutdown | HTTP server lacks graceful shutdown — in-flight requests aborted on SIGTERM |
| QUAL-021 | Medium | context-propagation | Notification dispatcher workers use context.Background() |
| QUAL-022 | Medium | context-propagation | ICMP resolver uses context.Background() — ignores per-check timeout |
| QUAL-023 | Medium | resource-leak | Hub.ClientCount() has a data race with Run() loop |
| QUAL-024 | Low | error-handling | Notification processJob half-handling pattern |
| QUAL-025 | Low | sqlc-consistency | Timescale and retention stores bypass sqlc layer |
| QUAL-026 | Low | context-propagation | Proto registry compilation uses context.Background() |
| QUAL-027 | Low | error-handling | Missing %w in reflect.go breaks error chain |
| QUAL-028 | Low | resource-leak | Hub.Run() goroutine not context-aware |
| QUAL-029 | Medium | shutdown | Shutdown sequence does not drain HTTP before stopping services |

**Findings by severity:**
- Critical: 0
- High: 1
- Medium: 4
- Low: 5
- Informational: 0
