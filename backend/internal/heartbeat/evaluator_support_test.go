package heartbeat_test

import (
	"context"
	"runtime"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
)

// countEvaluatorGoroutines returns the number of live goroutines currently
// executing inside the heartbeat Evaluator (its Run loop or worker pool). It
// reads the runtime stack dump and matches the evaluator's own frames, so it is
// unaffected by goroutines from other packages running in parallel.
func countEvaluatorGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	dump := string(buf[:n])

	count := 0
	for _, g := range strings.Split(dump, "\n\n") {
		if strings.Contains(g, "internal/heartbeat.(*Evaluator).worker") ||
			strings.Contains(g, "internal/heartbeat.(*Evaluator).Run") {
			count++
		}
	}
	return count
}

// newTestRegistry returns a fresh registry so each test's metric registration is
// isolated (avoids duplicate-collector panics across tests).
func newTestRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}

// recordingDispatcher captures DispatchMissedRun calls so tests can assert the
// once-per-episode guarantee and the previousState passed by the evaluator.
type recordingDispatcher struct {
	mu    sync.Mutex
	calls []dispatchCall
}

type dispatchCall struct {
	id            uuid.UUID
	previousState string
}

func (r *recordingDispatcher) DispatchMissedRun(_ context.Context, hb db.Heartbeat, previousState string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, dispatchCall{id: hb.ID, previousState: previousState})
}

func (r *recordingDispatcher) countFor(id uuid.UUID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if c.id == id {
			n++
		}
	}
	return n
}

func (r *recordingDispatcher) prevStatesFor(id uuid.UUID) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.calls {
		if c.id == id {
			out = append(out, c.previousState)
		}
	}
	return out
}

// recordingEventWriter captures WriteHeartbeatEvent calls so tests can assert the
// evaluator writes exactly one 'missed' event per missed-run episode.
type recordingEventWriter struct {
	mu    sync.Mutex
	calls []timescale.HeartbeatEvent
}

func (w *recordingEventWriter) WriteHeartbeatEvent(_ context.Context, ev timescale.HeartbeatEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, ev)
	return nil
}

func (w *recordingEventWriter) eventsFor(id uuid.UUID) []timescale.HeartbeatEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []timescale.HeartbeatEvent
	for _, c := range w.calls {
		if c.HeartbeatID == id {
			out = append(out, c)
		}
	}
	return out
}

func (w *recordingEventWriter) total() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.calls)
}

// fakeEvaluatorQueries is a DB-free implementation of the evaluator's narrowed
// Queries interface. markDownUpdates controls whether MarkHeartbeatDown reports a
// real up→down transition (returning a row) or the already-down case (ErrNoRows).
type fakeEvaluatorQueries struct {
	markDownUpdates bool
	row             db.Heartbeat
}

func (f *fakeEvaluatorQueries) ListDueHeartbeats(context.Context, int32) ([]db.Heartbeat, error) {
	// Always report the configured row as due so the worker pool calls markDown.
	return []db.Heartbeat{f.row}, nil
}

func (f *fakeEvaluatorQueries) CountHeartbeats(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeEvaluatorQueries) MarkHeartbeatDown(_ context.Context, id uuid.UUID) (db.Heartbeat, error) {
	if !f.markDownUpdates {
		// WHERE state='up' matched no row (already down) → sqlc returns ErrNoRows.
		return db.Heartbeat{}, pgx.ErrNoRows
	}
	updated := f.row
	updated.ID = id
	updated.State = "down"
	return updated, nil
}


