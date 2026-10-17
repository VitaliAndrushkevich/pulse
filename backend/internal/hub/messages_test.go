package hub

import (
	"encoding/json"
	"testing"
	"time"
)

// TestNewHeartbeatStatusMessageEnvelope asserts the constructor produces the
// correct envelope type and that the payload carries the heartbeat id and state.
func TestNewHeartbeatStatusMessageEnvelope(t *testing.T) {
	msg := NewHeartbeatStatusMessage("hb-1", "up", nil, nil)

	if msg.Type != TypeHeartbeatStatus {
		t.Fatalf("expected message type %q, got %q", TypeHeartbeatStatus, msg.Type)
	}

	payload, ok := msg.Payload.(HeartbeatStatusPayload)
	if !ok {
		t.Fatalf("payload is not HeartbeatStatusPayload, got %T", msg.Payload)
	}
	if payload.HeartbeatID != "hb-1" {
		t.Fatalf("expected heartbeat_id %q, got %q", "hb-1", payload.HeartbeatID)
	}
	if payload.State != "up" {
		t.Fatalf("expected state %q, got %q", "up", payload.State)
	}
	if payload.Timestamp == "" {
		t.Fatalf("timestamp must always be set")
	}
}

// TestNewHeartbeatStatusMessagePatchFieldSet verifies the diff/patch field set:
// nil optional timestamps are omitted from the serialized payload, while
// non-nil ones are included. heartbeat_id, state, and timestamp are always
// present.
func TestNewHeartbeatStatusMessagePatchFieldSet(t *testing.T) {
	lastSuccess := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	nextDeadline := time.Date(2026, 2, 3, 5, 5, 6, 0, time.UTC)

	tests := []struct {
		name            string
		lastSuccess     *time.Time
		nextDeadline    *time.Time
		wantLastSuccess bool
		wantNextDead    bool
	}{
		{
			name:            "both timestamps nil are omitted",
			lastSuccess:     nil,
			nextDeadline:    nil,
			wantLastSuccess: false,
			wantNextDead:    false,
		},
		{
			name:            "only last_success_at present",
			lastSuccess:     &lastSuccess,
			nextDeadline:    nil,
			wantLastSuccess: true,
			wantNextDead:    false,
		},
		{
			name:            "only next_expected_deadline present",
			lastSuccess:     nil,
			nextDeadline:    &nextDeadline,
			wantLastSuccess: false,
			wantNextDead:    true,
		},
		{
			name:            "both timestamps present",
			lastSuccess:     &lastSuccess,
			nextDeadline:    &nextDeadline,
			wantLastSuccess: true,
			wantNextDead:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewHeartbeatStatusMessage("hb-1", "down", tt.lastSuccess, tt.nextDeadline)

			data, err := json.Marshal(msg)
			if err != nil {
				t.Fatalf("failed to marshal message: %v", err)
			}

			var envelope map[string]any
			if err := json.Unmarshal(data, &envelope); err != nil {
				t.Fatalf("failed to unmarshal envelope: %v", err)
			}
			payload, ok := envelope["payload"].(map[string]any)
			if !ok {
				t.Fatalf("payload is not an object: %v", envelope["payload"])
			}

			// Always-present fields.
			for _, field := range []string{"heartbeat_id", "state", "timestamp"} {
				if _, exists := payload[field]; !exists {
					t.Fatalf("payload must contain %q, got: %s", field, string(data))
				}
			}

			if _, exists := payload["last_success_at"]; exists != tt.wantLastSuccess {
				t.Fatalf("last_success_at presence = %v, want %v: %s", exists, tt.wantLastSuccess, string(data))
			}
			if _, exists := payload["next_expected_deadline"]; exists != tt.wantNextDead {
				t.Fatalf("next_expected_deadline presence = %v, want %v: %s", exists, tt.wantNextDead, string(data))
			}
		})
	}
}

// TestNewHeartbeatStatusMessageRFC3339 asserts that every timestamp the
// constructor emits is RFC3339-formatted and reflects the input instants in UTC.
func TestNewHeartbeatStatusMessageRFC3339(t *testing.T) {
	lastSuccess := time.Date(2026, 2, 3, 4, 5, 6, 0, time.FixedZone("CET", 3600))
	nextDeadline := time.Date(2026, 2, 3, 5, 5, 6, 0, time.UTC)

	before := time.Now().UTC()
	msg := NewHeartbeatStatusMessage("hb-1", "up", &lastSuccess, &nextDeadline)
	after := time.Now().UTC()

	payload, ok := msg.Payload.(HeartbeatStatusPayload)
	if !ok {
		t.Fatalf("payload is not HeartbeatStatusPayload, got %T", msg.Payload)
	}

	// Server broadcast timestamp parses as RFC3339 and sits within bounds.
	ts, err := time.Parse(time.RFC3339Nano, payload.Timestamp)
	if err != nil {
		t.Fatalf("timestamp %q is not valid RFC3339: %v", payload.Timestamp, err)
	}
	if ts.Before(before) || ts.After(after) {
		t.Fatalf("timestamp %v is outside expected range [%v, %v]", ts, before, after)
	}

	// Optional timestamps parse as RFC3339 and equal the input instants.
	if payload.LastSuccessAt == nil {
		t.Fatalf("last_success_at must be set")
	}
	gotLast, err := time.Parse(time.RFC3339Nano, *payload.LastSuccessAt)
	if err != nil {
		t.Fatalf("last_success_at %q is not valid RFC3339: %v", *payload.LastSuccessAt, err)
	}
	if !gotLast.Equal(lastSuccess) {
		t.Fatalf("last_success_at instant %v does not equal input %v", gotLast, lastSuccess)
	}

	if payload.NextExpectedDeadline == nil {
		t.Fatalf("next_expected_deadline must be set")
	}
	gotNext, err := time.Parse(time.RFC3339Nano, *payload.NextExpectedDeadline)
	if err != nil {
		t.Fatalf("next_expected_deadline %q is not valid RFC3339: %v", *payload.NextExpectedDeadline, err)
	}
	if !gotNext.Equal(nextDeadline) {
		t.Fatalf("next_expected_deadline instant %v does not equal input %v", gotNext, nextDeadline)
	}
}
