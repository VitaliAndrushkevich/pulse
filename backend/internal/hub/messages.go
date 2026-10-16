package hub

import "time"

// MessageType constants for WebSocket message envelope.
const (
	// TypeMonitorStatus is sent when a monitor's check result changes.
	TypeMonitorStatus = "monitor_status"
	// TypeConnected is sent once to a client after successful connection.
	TypeConnected = "connected"
	// TypeMonitorTagsChanged is sent when a monitor's tags are modified.
	TypeMonitorTagsChanged = "monitor_tags_changed"
	// TypeHeartbeatStatus is sent when a heartbeat's observed state changes.
	TypeHeartbeatStatus = "heartbeat_status"
)

// MonitorStatusPayload is the diff/patch payload broadcast after each check.
// Only populated fields represent changed state — clients merge these patches
// into their local state rather than replacing entire monitor objects.
type MonitorStatusPayload struct {
	MonitorID        string  `json:"monitor_id"`
	State            string  `json:"state"`
	LatencyMs        int32   `json:"latency_ms"`
	StatusCode       *int32  `json:"status_code,omitempty"`
	SSLDaysRemaining *int32  `json:"ssl_days_remaining,omitempty"`
	Error            string  `json:"error,omitempty"`
	CheckedAt        string  `json:"checked_at"`
	Timestamp        string  `json:"timestamp"` // server time when broadcast was issued
}

// ConnectedPayload is sent to a client immediately after WebSocket upgrade.
type ConnectedPayload struct {
	ClientID  string `json:"client_id"`
	Timestamp string `json:"timestamp"`
}

// TagInfo represents a single key-value tag pair in WebSocket payloads.
type TagInfo struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MonitorTagsChangedPayload is broadcast when a monitor's tags are modified via the API.
type MonitorTagsChangedPayload struct {
	MonitorID string    `json:"monitor_id"`
	Tags      []TagInfo `json:"tags"`
	Timestamp string    `json:"timestamp"` // RFC3339 format
}

// NewMonitorStatusMessage creates a broadcast-ready Message from check result fields.
func NewMonitorStatusMessage(
	monitorID string,
	state string,
	latencyMs int32,
	statusCode *int32,
	sslDaysRemaining *int32,
	errMsg string,
	checkedAt time.Time,
) Message {
	return Message{
		Type: TypeMonitorStatus,
		Payload: MonitorStatusPayload{
			MonitorID:        monitorID,
			State:            state,
			LatencyMs:        latencyMs,
			StatusCode:       statusCode,
			SSLDaysRemaining: sslDaysRemaining,
			Error:            errMsg,
			CheckedAt:        checkedAt.UTC().Format(time.RFC3339Nano),
			Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
}

// HeartbeatStatusPayload is the diff/patch payload broadcast when a heartbeat's
// observed state changes (success ping, explicit-fail ping, missed-run detection,
// or recovery). Only state and the two timestamps are carried — clients merge the
// patch into local state rather than replacing the whole object. Optional
// timestamps are omitted when not set.
type HeartbeatStatusPayload struct {
	HeartbeatID          string  `json:"heartbeat_id"`
	State                string  `json:"state"`                            // up | down | paused | new
	LastSuccessAt        *string `json:"last_success_at,omitempty"`        // RFC3339
	NextExpectedDeadline *string `json:"next_expected_deadline,omitempty"` // RFC3339
	Timestamp            string  `json:"timestamp"`                        // server broadcast time
}

// NewHeartbeatStatusMessage creates a broadcast-ready heartbeat status patch.
// lastSuccess and nextDeadline are optional; nil timestamps are omitted from the
// payload so clients never clobber known values with absent ones.
func NewHeartbeatStatusMessage(id, state string, lastSuccess, nextDeadline *time.Time) Message {
	payload := HeartbeatStatusPayload{
		HeartbeatID: id,
		State:       state,
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if lastSuccess != nil {
		s := lastSuccess.UTC().Format(time.RFC3339Nano)
		payload.LastSuccessAt = &s
	}
	if nextDeadline != nil {
		s := nextDeadline.UTC().Format(time.RFC3339Nano)
		payload.NextExpectedDeadline = &s
	}
	return Message{
		Type:    TypeHeartbeatStatus,
		Payload: payload,
	}
}

// NewConnectedMessage creates the initial message sent after WS upgrade.
func NewConnectedMessage(clientID string) Message {
	return Message{
		Type: TypeConnected,
		Payload: ConnectedPayload{
			ClientID:  clientID,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
}

// NewMonitorTagsChangedMessage creates a broadcast message for tag modifications.
func NewMonitorTagsChangedMessage(monitorID string, tags []TagInfo) Message {
	return Message{
		Type: TypeMonitorTagsChanged,
		Payload: MonitorTagsChangedPayload{
			MonitorID: monitorID,
			Tags:      tags,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
}
