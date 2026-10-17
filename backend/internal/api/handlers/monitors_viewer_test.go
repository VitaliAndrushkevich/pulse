package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/gin-gonic/gin"
)

func TestMonitorViewerHidesFreeFormSettings(t *testing.T) {
	// Secret-bearing fields need not have recognizable names: arbitrary payload
	// keys and header names must be hidden as well as conventional passwords.
	settings := json.RawMessage(`{"headers":{"X-Custom":"secret"},"metadata":{"key":"secret"},"request_payload":"secret","custom":{"anything":"secret"}}`)
	monitor := db.Monitor{Settings: settings, Name: "Service", Target: "https://example.com"}
	for _, role := range []string{"read_only", "admin"} {
		t.Run(role, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("role", role)
			resp := monitorResponseForViewer(ctx, monitor, nil)
			if role == "read_only" && string(resp.Settings) != `{}` {
				t.Fatalf("viewer settings leaked: %s", resp.Settings)
			}
			if role == "admin" && string(resp.Settings) != string(settings) {
				t.Fatalf("admin settings changed: %s", resp.Settings)
			}
			if resp.Name != monitor.Name || resp.Target != monitor.Target {
				t.Fatal("monitor identity was lost")
			}
			if string(monitor.Settings) != string(settings) {
				t.Fatal("shared monitor settings were mutated")
			}
		})
	}
}
