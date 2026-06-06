package hub

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// **Validates: Requirements 8.3**
// Property 15: WebSocket Patch Minimality
// For any monitor_status message emitted during a regular health check, the
// payload SHALL NOT contain a tags field. Tags are only broadcast via
// monitor_tags_changed messages.

// TestPropertyMonitorStatusPayloadHasNoTagsField verifies structurally that
// MonitorStatusPayload does not declare a Tags field at all.
func TestPropertyMonitorStatusPayloadHasNoTagsField(t *testing.T) {
	typ := reflect.TypeOf(MonitorStatusPayload{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := field.Tag.Get("json")
		jsonName := strings.Split(jsonTag, ",")[0]
		if strings.EqualFold(field.Name, "Tags") || jsonName == "tags" {
			t.Fatalf("MonitorStatusPayload must NOT have a tags field, found: %s (json:%q)", field.Name, jsonTag)
		}
	}
}

// monitorStatusPayloadGen generates random MonitorStatusPayload values.
func monitorStatusPayloadGen() *rapid.Generator[MonitorStatusPayload] {
	return rapid.Custom(func(t *rapid.T) MonitorStatusPayload {
		monitorID := rapid.StringMatching(`[a-f0-9-]{36}`).Draw(t, "monitorID")
		state := rapid.SampledFrom([]string{"up", "down", "degraded"}).Draw(t, "state")
		latencyMs := rapid.Int32Range(0, 30000).Draw(t, "latencyMs")
		checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
		timestamp := time.Now().UTC().Format(time.RFC3339Nano)
		errMsg := rapid.SampledFrom([]string{"", "timeout", "connection refused"}).Draw(t, "error")

		var statusCode *int32
		if rapid.Bool().Draw(t, "hasStatusCode") {
			sc := rapid.Int32Range(100, 599).Draw(t, "statusCode")
			statusCode = &sc
		}

		var sslDays *int32
		if rapid.Bool().Draw(t, "hasSSLDays") {
			days := rapid.Int32Range(0, 365).Draw(t, "sslDays")
			sslDays = &days
		}

		return MonitorStatusPayload{
			MonitorID:        monitorID,
			State:            state,
			LatencyMs:        latencyMs,
			StatusCode:       statusCode,
			SSLDaysRemaining: sslDays,
			Error:            errMsg,
			CheckedAt:        checkedAt,
			Timestamp:        timestamp,
		}
	})
}

// TestPropertyMonitorStatusPayloadNoTagsInJSON uses property-based testing to
// generate random MonitorStatusPayload values and verify that their JSON
// serialization never contains a "tags" key.
func TestPropertyMonitorStatusPayloadNoTagsInJSON(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		payload := monitorStatusPayloadGen().Draw(t, "payload")

		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal MonitorStatusPayload: %v", err)
		}

		// Unmarshal into a generic map to check all top-level keys.
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("failed to unmarshal to map: %v", err)
		}

		if _, exists := m["tags"]; exists {
			t.Fatalf("monitor_status payload must NOT contain 'tags' key, got: %s", string(data))
		}
	})
}

// TestPropertyNewMonitorStatusMessageNoTagsInJSON verifies that the full
// Message envelope created by NewMonitorStatusMessage also never contains
// a tags field in the payload.
func TestPropertyNewMonitorStatusMessageNoTagsInJSON(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		monitorID := rapid.StringMatching(`[a-f0-9-]{36}`).Draw(t, "monitorID")
		state := rapid.SampledFrom([]string{"up", "down", "degraded"}).Draw(t, "state")
		latencyMs := rapid.Int32Range(0, 30000).Draw(t, "latencyMs")
		errMsg := rapid.SampledFrom([]string{"", "timeout", "connection refused"}).Draw(t, "error")
		checkedAt := time.Now().UTC()

		var statusCode *int32
		if rapid.Bool().Draw(t, "hasStatusCode") {
			sc := rapid.Int32Range(100, 599).Draw(t, "statusCode")
			statusCode = &sc
		}

		var sslDays *int32
		if rapid.Bool().Draw(t, "hasSSLDays") {
			days := rapid.Int32Range(0, 365).Draw(t, "sslDays")
			sslDays = &days
		}

		msg := NewMonitorStatusMessage(monitorID, state, latencyMs, statusCode, sslDays, errMsg, checkedAt)

		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("failed to marshal Message: %v", err)
		}

		// Parse the full envelope and check the payload object.
		var envelope map[string]interface{}
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Fatalf("failed to unmarshal envelope: %v", err)
		}

		payload, ok := envelope["payload"].(map[string]interface{})
		if !ok {
			t.Fatalf("payload is not an object: %v", envelope["payload"])
		}

		if _, exists := payload["tags"]; exists {
			t.Fatalf("monitor_status message payload must NOT contain 'tags' key, got: %s", string(data))
		}
	})
}

// validMonitorIDGen generates a random non-empty monitor ID string (UUID-like or arbitrary).
func validMonitorIDGen() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		// Generate a UUID-like string or a random non-empty string.
		length := rapid.IntRange(1, 64).Draw(t, "idLen")
		charset := "abcdefghijklmnopqrstuvwxyz0123456789-"
		buf := make([]byte, length)
		for i := range buf {
			buf[i] = charset[rapid.IntRange(0, len(charset)-1).Draw(t, "c")]
		}
		return string(buf)
	})
}

// validTagInfoGen generates a random TagInfo with non-empty key and value.
func validTagInfoGen() *rapid.Generator[TagInfo] {
	return rapid.Custom(func(t *rapid.T) TagInfo {
		keyLen := rapid.IntRange(1, 64).Draw(t, "keyLen")
		charset := "abcdefghijklmnopqrstuvwxyz0123456789_-"
		keyBuf := make([]byte, keyLen)
		for i := range keyBuf {
			keyBuf[i] = charset[rapid.IntRange(0, len(charset)-1).Draw(t, "kc")]
		}

		valLen := rapid.IntRange(1, 128).Draw(t, "valLen")
		valBuf := make([]byte, valLen)
		for i := range valBuf {
			valBuf[i] = byte(rapid.IntRange(0x20, 0x7E).Draw(t, "vc"))
		}

		return TagInfo{
			Key:   string(keyBuf),
			Value: string(valBuf),
		}
	})
}

// tagInfoSliceGen generates a random slice of TagInfo (0 to 20 elements).
func tagInfoSliceGen() *rapid.Generator[[]TagInfo] {
	return rapid.Custom(func(t *rapid.T) []TagInfo {
		size := rapid.IntRange(0, 20).Draw(t, "numTags")
		tags := make([]TagInfo, size)
		for i := range tags {
			tags[i] = validTagInfoGen().Draw(t, "tag")
		}
		return tags
	})
}

// TestPropertyTagChangeNotificationCompleteness verifies Property 16: Tag Change
// Notification Completeness.
//
// Generate random tag modifications; verify broadcast contains monitor_id, full
// tags array, and timestamp.
//
// **Validates: Requirements 8.1, 8.2**
func TestPropertyTagChangeNotificationCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		monitorID := validMonitorIDGen().Draw(t, "monitorID")
		tags := tagInfoSliceGen().Draw(t, "tags")

		before := time.Now().UTC()
		msg := NewMonitorTagsChangedMessage(monitorID, tags)
		after := time.Now().UTC()

		// 1. Message type must be "monitor_tags_changed".
		if msg.Type != TypeMonitorTagsChanged {
			t.Fatalf("expected message type %q, got %q", TypeMonitorTagsChanged, msg.Type)
		}

		// 2. Extract and verify payload.
		payload, ok := msg.Payload.(MonitorTagsChangedPayload)
		if !ok {
			t.Fatalf("payload is not MonitorTagsChangedPayload, got %T", msg.Payload)
		}

		// 3. Payload contains non-empty monitor_id matching the input.
		if payload.MonitorID == "" {
			t.Fatalf("payload monitor_id is empty")
		}
		if payload.MonitorID != monitorID {
			t.Fatalf("payload monitor_id %q does not match input %q", payload.MonitorID, monitorID)
		}

		// 4. Payload contains full tags array matching the input.
		if len(payload.Tags) != len(tags) {
			t.Fatalf("payload tags length %d does not match input tags length %d",
				len(payload.Tags), len(tags))
		}
		for i, tag := range tags {
			if payload.Tags[i].Key != tag.Key || payload.Tags[i].Value != tag.Value {
				t.Fatalf("tag at index %d mismatch: got {%q, %q}, want {%q, %q}",
					i, payload.Tags[i].Key, payload.Tags[i].Value, tag.Key, tag.Value)
			}
		}

		// 5. Payload contains a non-empty timestamp in RFC3339 format.
		if payload.Timestamp == "" {
			t.Fatalf("payload timestamp is empty")
		}
		ts, err := time.Parse(time.RFC3339Nano, payload.Timestamp)
		if err != nil {
			t.Fatalf("payload timestamp %q is not valid RFC3339: %v", payload.Timestamp, err)
		}

		// Verify timestamp is within reasonable bounds (between before and after).
		if ts.Before(before) || ts.After(after) {
			t.Fatalf("timestamp %v is outside expected range [%v, %v]", ts, before, after)
		}

		// 6. Verify the message serializes correctly to JSON with all required fields.
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("failed to marshal message to JSON: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("failed to unmarshal message JSON: %v", err)
		}

		// Verify top-level type field.
		if parsed["type"] != "monitor_tags_changed" {
			t.Fatalf("JSON type field is %v, expected 'monitor_tags_changed'", parsed["type"])
		}

		// Verify payload contains required fields.
		payloadMap, ok := parsed["payload"].(map[string]interface{})
		if !ok {
			t.Fatalf("JSON payload is not an object")
		}
		if _, exists := payloadMap["monitor_id"]; !exists {
			t.Fatalf("JSON payload missing 'monitor_id' field")
		}
		if _, exists := payloadMap["tags"]; !exists {
			t.Fatalf("JSON payload missing 'tags' field")
		}
		if _, exists := payloadMap["timestamp"]; !exists {
			t.Fatalf("JSON payload missing 'timestamp' field")
		}
	})
}
