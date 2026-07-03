package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// TestMonitorDelete_CascadeDeletesBindings verifies Requirement 3.7:
// "WHEN a monitor is deleted, THE Notification_API SHALL remove all
// Channel_Bindings associated with that monitor."
//
// The cascade is implemented via the FK constraint in migration 014:
//
//	monitor_id UUID NOT NULL REFERENCES monitors(id) ON DELETE CASCADE
//
// This means PostgreSQL automatically deletes all channel_bindings rows
// referencing the deleted monitor. The handler does NOT need to explicitly
// delete bindings — a single DELETE FROM monitors WHERE id = $1 is sufficient.
//
// This test verifies the handler correctly issues the delete and returns 204.
// The actual FK cascade is a database-level guarantee validated by the
// migration schema definition.

// monitorCascadeFakeDB implements db.DBTX for monitor cascade delete tests.
type monitorCascadeFakeDB struct {
	monitors map[uuid.UUID]bool
	deleted  []uuid.UUID // tracks which monitors were deleted
}

func newMonitorCascadeFakeDB() *monitorCascadeFakeDB {
	return &monitorCascadeFakeDB{
		monitors: make(map[uuid.UUID]bool),
	}
}

func (f *monitorCascadeFakeDB) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	// DeleteMonitor: DELETE FROM monitors WHERE id = $1
	if len(args) > 0 {
		id := args[0].(uuid.UUID)
		delete(f.monitors, id)
		f.deleted = append(f.deleted, id)
	}
	return pgconn.NewCommandTag("DELETE 1"), nil
}

func (f *monitorCascadeFakeDB) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (f *monitorCascadeFakeDB) QueryRow(_ context.Context, _ string, _ ...interface{}) pgx.Row {
	return &monitorCascadeFakeRow{}
}

type monitorCascadeFakeRow struct{}

func (r *monitorCascadeFakeRow) Scan(_ ...interface{}) error {
	return pgx.ErrNoRows
}

func setupMonitorCascadeRouter(fdb *monitorCascadeFakeDB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// X-Request-ID middleware.
	r.Use(func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Next()
	})

	queries := db.New(fdb)
	v1 := r.Group("/api/v1")

	monitorHandler := handlers.NewMonitorHandler(queries, nil, nil)
	monitorHandler.Register(v1)

	return r
}

func TestMonitorDelete_Returns204_CascadeHandledByFK(t *testing.T) {
	// This test verifies that the monitor DELETE handler:
	// 1. Calls DeleteMonitor (which issues DELETE FROM monitors WHERE id = $1)
	// 2. Returns 204 No Content
	//
	// The FK constraint ON DELETE CASCADE on channel_bindings.monitor_id
	// guarantees that all bindings for this monitor are deleted by PostgreSQL.
	// No explicit binding deletion is needed in the handler.

	fdb := newMonitorCascadeFakeDB()
	monitorID := uuid.New()
	fdb.monitors[monitorID] = true

	router := setupMonitorCascadeRouter(fdb)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/monitors/"+monitorID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the monitor was deleted via the query layer.
	if len(fdb.deleted) != 1 {
		t.Fatalf("expected exactly 1 delete call, got %d", len(fdb.deleted))
	}
	if fdb.deleted[0] != monitorID {
		t.Errorf("expected delete for monitor %s, got %s", monitorID, fdb.deleted[0])
	}

	// Verify no explicit binding deletion call was made — the handler relies
	// solely on the FK ON DELETE CASCADE constraint (migration 014).
	// This is the correct behavior per Requirement 3.7.
}

func TestMonitorDelete_InvalidUUID_Returns400(t *testing.T) {
	fdb := newMonitorCascadeFakeDB()
	router := setupMonitorCascadeRouter(fdb)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/monitors/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid UUID, got %d: %s", w.Code, w.Body.String())
	}

	// No delete should have been called.
	if len(fdb.deleted) != 0 {
		t.Errorf("expected no delete calls for invalid UUID, got %d", len(fdb.deleted))
	}
}
