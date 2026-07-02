package tools

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"pgregory.net/rapid"
)

// --- Generators ---

// genTag generates a random Tag with non-empty key and value.
func genTag() *rapid.Generator[pulseapi.Tag] {
	return rapid.Custom(func(t *rapid.T) pulseapi.Tag {
		key := rapid.StringMatching(`[a-z][a-z0-9_]{0,19}`).Draw(t, "tagKey")
		value := rapid.StringMatching(`[a-zA-Z0-9._-]{1,30}`).Draw(t, "tagValue")
		return pulseapi.Tag{Key: key, Value: value}
	})
}

// genTime generates a random time within a reasonable range.
func genTime() *rapid.Generator[time.Time] {
	return rapid.Custom(func(t *rapid.T) time.Time {
		// Random unix timestamp between 2020-01-01 and 2030-01-01.
		sec := rapid.Int64Range(1577836800, 1893456000).Draw(t, "unixSec")
		return time.Unix(sec, 0).UTC()
	})
}

// genOptionalTime generates a *time.Time that may be nil.
func genOptionalTime() *rapid.Generator[*time.Time] {
	return rapid.Custom(func(t *rapid.T) *time.Time {
		isNil := rapid.Bool().Draw(t, "isNil")
		if isNil {
			return nil
		}
		tm := genTime().Draw(t, "time")
		return &tm
	})
}

// genMonitor generates a random pulseapi.Monitor with arbitrary field values.
func genMonitor() *rapid.Generator[pulseapi.Monitor] {
	return rapid.Custom(func(t *rapid.T) pulseapi.Monitor {
		id := rapid.StringMatching(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`).Draw(t, "id")
		name := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9 _-]{0,49}`).Draw(t, "name")
		monType := rapid.SampledFrom([]string{"http", "https", "http3", "tcp", "udp", "websocket", "grpc", "dns", "icmp", "smtp"}).Draw(t, "type")
		target := rapid.StringMatching(`[a-z0-9.:/]{5,50}`).Draw(t, "target")
		intervalSec := rapid.IntRange(10, 3600).Draw(t, "interval")
		timeoutSec := rapid.IntRange(1, 60).Draw(t, "timeout")
		status := rapid.SampledFrom([]string{"up", "down", "pending"}).Draw(t, "status")
		state := rapid.SampledFrom([]string{"active", "paused"}).Draw(t, "state")
		tagCount := rapid.IntRange(0, 5).Draw(t, "tagCount")
		tags := make([]pulseapi.Tag, tagCount)
		for i := range tags {
			tags[i] = genTag().Draw(t, "tag")
		}
		createdAt := genTime().Draw(t, "createdAt")
		updatedAt := genTime().Draw(t, "updatedAt")
		lastCheckedAt := genOptionalTime().Draw(t, "lastCheckedAt")
		nextCheckAt := genOptionalTime().Draw(t, "nextCheckAt")

		return pulseapi.Monitor{
			ID:              id,
			Name:            name,
			Type:            monType,
			Target:          target,
			IntervalSeconds: intervalSec,
			TimeoutSeconds:  timeoutSec,
			Status:          status,
			State:           state,
			Tags:            tags,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			LastCheckedAt:   lastCheckedAt,
			NextCheckAt:     nextCheckAt,
		}
	})
}

// --- Property 9: Monitor projection preserves non-secret fields faithfully ---
// **Validates: Requirements 5.1**
//
// For any Monitor struct with arbitrary field values, the output of projectMonitor
// preserves all non-secret fields (id, name, type, target, interval_seconds,
// timeout_seconds, status, state, tags, created_at, updated_at, last_checked_at,
// next_check_at) faithfully without alteration.
func TestProperty9_MonitorProjectionPreservesNonSecretFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m := genMonitor().Draw(t, "monitor")
		out := projectMonitor(m)

		// Scalar string fields must pass through unchanged.
		if out.ID != m.ID {
			t.Fatalf("ID mismatch: got %q, want %q", out.ID, m.ID)
		}
		if out.Name != m.Name {
			t.Fatalf("Name mismatch: got %q, want %q", out.Name, m.Name)
		}
		if out.Type != m.Type {
			t.Fatalf("Type mismatch: got %q, want %q", out.Type, m.Type)
		}
		if out.Target != m.Target {
			t.Fatalf("Target mismatch: got %q, want %q", out.Target, m.Target)
		}
		if out.Status != m.Status {
			t.Fatalf("Status mismatch: got %q, want %q", out.Status, m.Status)
		}
		if out.State != m.State {
			t.Fatalf("State mismatch: got %q, want %q", out.State, m.State)
		}

		// Numeric fields must pass through unchanged.
		if out.IntervalSeconds != m.IntervalSeconds {
			t.Fatalf("IntervalSeconds mismatch: got %d, want %d", out.IntervalSeconds, m.IntervalSeconds)
		}
		if out.TimeoutSeconds != m.TimeoutSeconds {
			t.Fatalf("TimeoutSeconds mismatch: got %d, want %d", out.TimeoutSeconds, m.TimeoutSeconds)
		}

		// Tags must be faithfully represented as "key:value" strings.
		if len(out.Tags) != len(m.Tags) {
			t.Fatalf("Tags length mismatch: got %d, want %d", len(out.Tags), len(m.Tags))
		}
		for i, tag := range m.Tags {
			expected := fmt.Sprintf("%s:%s", tag.Key, tag.Value)
			if out.Tags[i] != expected {
				t.Fatalf("Tags[%d] mismatch: got %q, want %q", i, out.Tags[i], expected)
			}
		}

		// Timestamps must be formatted as RFC3339 from the original time values.
		expectedCreatedAt := m.CreatedAt.Format(time.RFC3339)
		if out.CreatedAt != expectedCreatedAt {
			t.Fatalf("CreatedAt mismatch: got %q, want %q", out.CreatedAt, expectedCreatedAt)
		}
		expectedUpdatedAt := m.UpdatedAt.Format(time.RFC3339)
		if out.UpdatedAt != expectedUpdatedAt {
			t.Fatalf("UpdatedAt mismatch: got %q, want %q", out.UpdatedAt, expectedUpdatedAt)
		}

		// Optional timestamp fields: present exactly when the source is non-nil,
		// and their RFC3339 value must match.
		if m.LastCheckedAt == nil {
			if out.LastCheckedAt != nil {
				t.Fatalf("LastCheckedAt should be nil when source is nil, got %q", *out.LastCheckedAt)
			}
		} else {
			if out.LastCheckedAt == nil {
				t.Fatal("LastCheckedAt should not be nil when source is non-nil")
			}
			expected := m.LastCheckedAt.Format(time.RFC3339)
			if *out.LastCheckedAt != expected {
				t.Fatalf("LastCheckedAt mismatch: got %q, want %q", *out.LastCheckedAt, expected)
			}
		}

		if m.NextCheckAt == nil {
			if out.NextCheckAt != nil {
				t.Fatalf("NextCheckAt should be nil when source is nil, got %q", *out.NextCheckAt)
			}
		} else {
			if out.NextCheckAt == nil {
				t.Fatal("NextCheckAt should not be nil when source is non-nil")
			}
			expected := m.NextCheckAt.Format(time.RFC3339)
			if *out.NextCheckAt != expected {
				t.Fatalf("NextCheckAt mismatch: got %q, want %q", *out.NextCheckAt, expected)
			}
		}

		// Empty tags must produce an empty slice (not nil).
		if len(m.Tags) == 0 && out.Tags == nil {
			t.Fatal("Tags should be empty slice [], not nil, when source has no tags")
		}

		// Verify no extra content injected — tag strings should only contain
		// "key:value" format with no additional separators or data.
		for _, tagStr := range out.Tags {
			parts := strings.SplitN(tagStr, ":", 2)
			if len(parts) != 2 {
				t.Fatalf("Tag string %q is not in key:value format", tagStr)
			}
		}
	})
}
