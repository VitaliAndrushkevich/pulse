package pulseapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClient_AuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("X-Request-ID", "req-123")
		json.NewEncoder(w).Encode(wireMonitor{ID: "abc", Name: "test"})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "my-secret-token", 5*time.Second)
	_, err := client.GetMonitor(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer my-secret-token")
	}
}

func TestHTTPClient_ErrorEnvelopeParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req-456")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "MONITOR_NOT_FOUND",
				"message": "Monitor with id abc not found",
			},
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	_, err := client.GetMonitor(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pulseErr *PulseError
	if !errors.As(err, &pulseErr) {
		t.Fatalf("expected *PulseError, got %T: %v", err, err)
	}
	if pulseErr.Code != "MONITOR_NOT_FOUND" {
		t.Errorf("Code = %q, want %q", pulseErr.Code, "MONITOR_NOT_FOUND")
	}
	if pulseErr.Message != "Monitor with id abc not found" {
		t.Errorf("Message = %q, want %q", pulseErr.Message, "Monitor with id abc not found")
	}
	if pulseErr.RequestID != "req-456" {
		t.Errorf("RequestID = %q, want %q", pulseErr.RequestID, "req-456")
	}
	if pulseErr.HTTPStatus != 404 {
		t.Errorf("HTTPStatus = %d, want %d", pulseErr.HTTPStatus, 404)
	}
}

func TestHTTPClient_SyntheticErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req-789")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("something went wrong"))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	_, err := client.GetMonitor(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pulseErr *PulseError
	if !errors.As(err, &pulseErr) {
		t.Fatalf("expected *PulseError, got %T: %v", err, err)
	}
	if pulseErr.Code != "INTERNAL_ERROR" {
		t.Errorf("Code = %q, want %q", pulseErr.Code, "INTERNAL_ERROR")
	}
	if pulseErr.RequestID != "req-789" {
		t.Errorf("RequestID = %q, want %q", pulseErr.RequestID, "req-789")
	}
}

func TestHTTPClient_401Handling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req-unauth")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "UNAUTHORIZED",
				"message": "Invalid or revoked API token",
			},
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "bad-token", 5*time.Second)
	_, err := client.GetMonitor(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pulseErr *PulseError
	if !errors.As(err, &pulseErr) {
		t.Fatalf("expected *PulseError, got %T: %v", err, err)
	}
	if pulseErr.Code != "UNAUTHORIZED" {
		t.Errorf("Code = %q, want %q", pulseErr.Code, "UNAUTHORIZED")
	}
	if pulseErr.HTTPStatus != 401 {
		t.Errorf("HTTPStatus = %d, want %d", pulseErr.HTTPStatus, 401)
	}
	if pulseErr.RequestID != "req-unauth" {
		t.Errorf("RequestID = %q, want %q", pulseErr.RequestID, "req-unauth")
	}
}

func TestHTTPClient_XRequestIDPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "trace-abc-123")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "VALIDATION_ERROR",
				"message": "Invalid parameter",
			},
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	_, err := client.ListMonitors(context.Background(), MonitorQuery{Page: 1, Limit: 10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pulseErr *PulseError
	if !errors.As(err, &pulseErr) {
		t.Fatalf("expected *PulseError, got %T: %v", err, err)
	}
	if pulseErr.RequestID != "trace-abc-123" {
		t.Errorf("RequestID = %q, want %q", pulseErr.RequestID, "trace-abc-123")
	}
}

func TestHTTPClient_ConnectivityError_ConnectionRefused(t *testing.T) {
	// Use a port that is guaranteed not listening.
	client := NewHTTPClient("http://127.0.0.1:1", "token", 2*time.Second)
	_, err := client.GetMonitor(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var connErr *ConnectivityError
	if !errors.As(err, &connErr) {
		t.Fatalf("expected *ConnectivityError, got %T: %v", err, err)
	}
	if connErr.Reason != "connection_refused" && connErr.Reason != "dial_error" {
		t.Errorf("Reason = %q, want connection_refused or dial_error", connErr.Reason)
	}
}

func TestHTTPClient_ConnectivityError_Timeout(t *testing.T) {
	// Server that responds slowly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 100*time.Millisecond)
	_, err := client.GetMonitor(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var connErr *ConnectivityError
	if !errors.As(err, &connErr) {
		t.Fatalf("expected *ConnectivityError, got %T: %v", err, err)
	}
	if connErr.Reason != "timeout" {
		t.Errorf("Reason = %q, want timeout", connErr.Reason)
	}
}

func TestHTTPClient_ListMonitors_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/monitors" {
			t.Errorf("path = %q, want /monitors", r.URL.Path)
		}
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("page = %q, want 2", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("limit = %q, want 10", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("type") != "http" {
			t.Errorf("type = %q, want http", r.URL.Query().Get("type"))
		}

		w.Header().Set("X-Request-ID", "req-list")
		json.NewEncoder(w).Encode(listMonitorsEnvelope{
			Data: []wireMonitor{
				{ID: "m1", Name: "Web", Type: "http", Target: "https://example.com", Status: "up", State: "active", CreatedAt: now, UpdatedAt: now},
			},
			Total:      15,
			Page:       2,
			Limit:      10,
			TotalPages: 2,
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	page, err := client.ListMonitors(context.Background(), MonitorQuery{
		Type:  "http",
		Page:  2,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Monitors) != 1 {
		t.Fatalf("len(Monitors) = %d, want 1", len(page.Monitors))
	}
	if page.Monitors[0].ID != "m1" {
		t.Errorf("ID = %q, want m1", page.Monitors[0].ID)
	}
	if page.Total != 15 {
		t.Errorf("Total = %d, want 15", page.Total)
	}
	if page.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", page.TotalPages)
	}
}

func TestHTTPClient_ListMonitors_TagQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tags := r.URL.Query()["tag"]
		if len(tags) != 2 || tags[0] != "env:prod" || tags[1] != "team:sre" {
			t.Errorf("tags = %v, want [env:prod team:sre]", tags)
		}
		w.Header().Set("X-Request-ID", "req-tags")
		json.NewEncoder(w).Encode(listMonitorsEnvelope{
			Data:       []wireMonitor{},
			Total:      0,
			Page:       1,
			Limit:      50,
			TotalPages: 0,
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	page, err := client.ListMonitors(context.Background(), MonitorQuery{
		Tags:  []string{"env:prod", "team:sre"},
		Page:  1,
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Monitors) != 0 {
		t.Errorf("len(Monitors) = %d, want 0", len(page.Monitors))
	}
}

func TestHTTPClient_GetMonitorStats_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/monitors/m1/stats" {
			t.Errorf("path = %q, want /monitors/m1/stats", r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req-stats")
		json.NewEncoder(w).Encode(wireMonitorStats{
			MonitorID: "m1",
			Uptime24h: wireUptimeWindow{UptimePercent: 99.95},
			Uptime30d: wireUptimeWindow{UptimePercent: 99.80},
			LastError: &wireLastError{Error: "timeout", CheckedAt: "2025-01-01T12:00:00Z"},
			SSL:       &wireSSL{ExpiresAt: "2025-02-01", DaysRemaining: 30},
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	stats, err := client.GetMonitorStats(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.MonitorID != "m1" {
		t.Errorf("MonitorID = %q, want m1", stats.MonitorID)
	}
	if stats.UptimePercent7d != 99.95 {
		t.Errorf("UptimePercent7d = %f, want 99.95", stats.UptimePercent7d)
	}
	if stats.LastError == nil {
		t.Fatal("LastError is nil")
	}
	if stats.LastError.Message != "timeout" {
		t.Errorf("LastError.Message = %q, want timeout", stats.LastError.Message)
	}
	if stats.SSL == nil {
		t.Fatal("SSL is nil")
	}
	if stats.SSL.DaysRemaining != 30 {
		t.Errorf("DaysRemaining = %d, want 30", stats.SSL.DaysRemaining)
	}
}

func TestHTTPClient_GetMonitorHistory_Success(t *testing.T) {
	from := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	to := time.Now().UTC().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/monitors/m1/history" {
			t.Errorf("path = %q, want /monitors/m1/history", r.URL.Path)
		}
		if r.URL.Query().Get("from") == "" {
			t.Error("from param is missing")
		}
		if r.URL.Query().Get("to") == "" {
			t.Error("to param is missing")
		}
		w.Header().Set("X-Request-ID", "req-hist")
		latency := int32(42)
		status := int32(200)
		json.NewEncoder(w).Encode(historyEnvelope{
			MonitorID: "m1",
			From:      from.Format(time.RFC3339),
			To:        to.Format(time.RFC3339),
			Truncated: false,
			Points: []wireHistoryPt{
				{State: "up", LatencyMs: &latency, StatusCode: &status, CheckedAt: from.Add(time.Hour).Format(time.RFC3339)},
			},
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	hist, err := client.GetMonitorHistory(context.Background(), "m1", TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hist.MonitorID != "m1" {
		t.Errorf("MonitorID = %q, want m1", hist.MonitorID)
	}
	if len(hist.Points) != 1 {
		t.Fatalf("len(Points) = %d, want 1", len(hist.Points))
	}
	if hist.Points[0].State != "up" {
		t.Errorf("State = %q, want up", hist.Points[0].State)
	}
	if *hist.Points[0].LatencyMs != 42 {
		t.Errorf("LatencyMs = %d, want 42", *hist.Points[0].LatencyMs)
	}
}

func TestHTTPClient_ListIncidents_Global(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/incidents" {
			t.Errorf("path = %q, want /incidents", r.URL.Path)
		}
		if r.URL.Query().Get("status") != "open" {
			t.Errorf("status = %q, want open", r.URL.Query().Get("status"))
		}
		w.Header().Set("X-Request-ID", "req-inc")
		json.NewEncoder(w).Encode(listIncidentsEnvelope{
			Data: []wireIncident{
				{ID: "inc-1", MonitorID: "m1", StartedAt: now},
			},
			Total:      1,
			Page:       1,
			Limit:      20,
			TotalPages: 1,
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	page, err := client.ListIncidents(context.Background(), IncidentQuery{
		OpenOnly: true,
		Page:     1,
		Limit:    20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Incidents) != 1 {
		t.Fatalf("len(Incidents) = %d, want 1", len(page.Incidents))
	}
	if page.Incidents[0].Status != "open" {
		t.Errorf("Status = %q, want open", page.Incidents[0].Status)
	}
}

func TestHTTPClient_ListIncidents_PerMonitor(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/monitors/m1/incidents" {
			t.Errorf("path = %q, want /monitors/m1/incidents", r.URL.Path)
		}
		// Per-monitor endpoint should NOT have status param.
		if r.URL.Query().Get("status") != "" {
			t.Errorf("status should not be sent for per-monitor, got %q", r.URL.Query().Get("status"))
		}
		w.Header().Set("X-Request-ID", "req-inc-mon")
		json.NewEncoder(w).Encode(listIncidentsEnvelope{
			Data: []wireIncident{
				{ID: "inc-2", MonitorID: "m1", StartedAt: now, ResolvedAt: &now},
			},
			Total:      1,
			Page:       1,
			Limit:      20,
			TotalPages: 1,
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	page, err := client.ListIncidents(context.Background(), IncidentQuery{
		MonitorID: "m1",
		Page:      1,
		Limit:     20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Incidents) != 1 {
		t.Fatalf("len(Incidents) = %d, want 1", len(page.Incidents))
	}
	if page.Incidents[0].ResolvedAt == nil {
		t.Error("ResolvedAt should not be nil for resolved incident")
	}
}

func TestHTTPClient_CreateMonitor_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/monitors" {
			t.Errorf("path = %q, want /monitors", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}

		var body createMonitorRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Name != "my-monitor" {
			t.Errorf("Name = %q, want my-monitor", body.Name)
		}
		if body.Type != "http" {
			t.Errorf("Type = %q, want http", body.Type)
		}

		w.Header().Set("X-Request-ID", "req-create")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(wireMonitor{
			ID:              "new-id",
			Name:            "my-monitor",
			Type:            "http",
			Target:          "https://example.com",
			IntervalSeconds: 60,
			TimeoutSeconds:  10,
			Status:          "pending",
			State:           "active",
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 5*time.Second)
	monitor, err := client.CreateMonitor(context.Background(), CreateMonitorInput{
		Type:   "http",
		Name:   "my-monitor",
		Target: "https://example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if monitor.ID != "new-id" {
		t.Errorf("ID = %q, want new-id", monitor.ID)
	}
	if monitor.Status != "pending" {
		t.Errorf("Status = %q, want pending", monitor.Status)
	}
	if monitor.IntervalSeconds != 60 {
		t.Errorf("IntervalSeconds = %d, want 60", monitor.IntervalSeconds)
	}
}

func TestHTTPClient_PerRequestTimeout(t *testing.T) {
	// Verify that context.WithTimeout is applied per-request.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Simulate slow response only for the first call.
		if callCount == 1 {
			time.Sleep(300 * time.Millisecond)
		}
		w.Header().Set("X-Request-ID", "req-timeout")
		json.NewEncoder(w).Encode(wireMonitor{ID: "m1", Name: "test"})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "token", 100*time.Millisecond)

	// First call should timeout.
	_, err := client.GetMonitor(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected timeout error for first call")
	}
	var connErr *ConnectivityError
	if !errors.As(err, &connErr) {
		t.Fatalf("expected *ConnectivityError, got %T: %v", err, err)
	}

	// Second call should succeed (server responds quickly).
	m, err := client.GetMonitor(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if m.ID != "m1" {
		t.Errorf("ID = %q, want m1", m.ID)
	}
}
