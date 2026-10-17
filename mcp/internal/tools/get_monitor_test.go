package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

func TestHandleGetMonitor_AccountRestrictions(t *testing.T) {
	for _, code := range []string{"FORBIDDEN", "PASSWORD_CHANGE_REQUIRED"} {
		t.Run(code, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-ID", "account-policy")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "account restricted"}})
			}))
			defer srv.Close()
			client := pulseapi.NewHTTPClient(srv.URL, "token", time.Second)
			out, err := HandleGetMonitor(context.Background(), Deps{Client: client}, GetMonitorInput{Monitor: "00000000-0000-0000-0000-000000000001"})
			var policyErr *mcperr.MCPError
			if out != nil || !errors.As(err, &policyErr) {
				t.Fatalf("expected account policy error, got output=%v error=%v", out, err)
			}
			if policyErr.Code != code || policyErr.RequestID != "account-policy" {
				t.Fatalf("account error lost details: %+v", policyErr)
			}
		})
	}
}

func TestHandleGetMonitor_RedactedSettings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000001","name":"API","type":"http","target":"https://example.com","settings":{},"state":"active","status":"up"}`))
	}))
	defer srv.Close()
	client := pulseapi.NewHTTPClient(srv.URL, "token", time.Second)
	out, err := HandleGetMonitor(context.Background(), Deps{Client: client}, GetMonitorInput{Monitor: "00000000-0000-0000-0000-000000000001"})
	if err != nil || out == nil || out.Name != "API" || out.Status != "up" {
		t.Fatalf("redacted monitor should remain readable: output=%v error=%v", out, err)
	}
}
