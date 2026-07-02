package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"pgregory.net/rapid"
)

// --- Generators ---

// genHistoryState generates a valid monitor state string.
func genHistoryState() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"up", "down", "pending"})
}

// genOptionalInt32 generates an optional *int32 value.
func genOptionalInt32() *rapid.Generator[*int32] {
	return rapid.Custom(func(t *rapid.T) *int32 {
		if rapid.Bool().Draw(t, "hasValue") {
			v := int32(rapid.IntRange(0, 10000).Draw(t, "value"))
			return &v
		}
		return nil
	})
}

// genOptionalString generates an optional *string value.
func genOptionalString() *rapid.Generator[*string] {
	return rapid.Custom(func(t *rapid.T) *string {
		if rapid.Bool().Draw(t, "hasValue") {
			s := rapid.StringMatching(`[a-z ]{1,30}`).Draw(t, "strValue")
			return &s
		}
		return nil
	})
}

// genHistoryPoint generates a HistoryPoint with a CheckedAt within the given range.
func genHistoryPoint(from, to time.Time) *rapid.Generator[pulseapi.HistoryPoint] {
	return rapid.Custom(func(t *rapid.T) pulseapi.HistoryPoint {
		// Generate a timestamp within [from, to].
		offset := rapid.Int64Range(0, int64(to.Sub(from))).Draw(t, "offset")
		checkedAt := from.Add(time.Duration(offset))

		return pulseapi.HistoryPoint{
			State:      genHistoryState().Draw(t, "state"),
			LatencyMs:  genOptionalInt32().Draw(t, "latencyMs"),
			StatusCode: genOptionalInt32().Draw(t, "statusCode"),
			Error:      genOptionalString().Draw(t, "error"),
			CheckedAt:  checkedAt,
		}
	})
}

// genValidTimeRange generates a from/to pair where from <= to with reasonable values.
func genValidTimeRange() *rapid.Generator[[2]time.Time] {
	return rapid.Custom(func(t *rapid.T) [2]time.Time {
		// Base time: sometime in 2025.
		baseUnix := int64(1735689600) // 2025-01-01T00:00:00Z
		fromUnix := rapid.Int64Range(baseUnix, baseUnix+7*24*3600).Draw(t, "fromUnix")
		// To is from + [1 second, 7 days].
		duration := rapid.Int64Range(1, 7*24*3600).Draw(t, "durationSec")
		toUnix := fromUnix + duration

		from := time.Unix(fromUnix, 0).UTC()
		to := time.Unix(toUnix, 0).UTC()
		return [2]time.Time{from, to}
	})
}

// genInvertedTimeRange generates a from/to pair where from > to (strictly).
func genInvertedTimeRange() *rapid.Generator[[2]time.Time] {
	return rapid.Custom(func(t *rapid.T) [2]time.Time {
		baseUnix := int64(1735689600) // 2025-01-01T00:00:00Z
		toUnix := rapid.Int64Range(baseUnix, baseUnix+7*24*3600).Draw(t, "toUnix")
		// From is strictly after to: to + [1 second, 7 days].
		gap := rapid.Int64Range(1, 7*24*3600).Draw(t, "gapSec")
		fromUnix := toUnix + gap

		from := time.Unix(fromUnix, 0).UTC()
		to := time.Unix(toUnix, 0).UTC()
		return [2]time.Time{from, to}
	})
}

// genWhitespaceOnly generates a string that is empty or whitespace-only.
func genWhitespaceOnly() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		length := rapid.IntRange(0, 10).Draw(t, "wsLen")
		ws := []rune{' ', '\t', '\n', '\r'}
		var b strings.Builder
		for i := 0; i < length; i++ {
			idx := rapid.IntRange(0, len(ws)-1).Draw(t, "wsIdx")
			b.WriteRune(ws[idx])
		}
		return b.String()
	})
}

// genValidUUID generates a valid UUID string (8-4-4-4-12 hex format).
func genValidUUID() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		hexChars := "0123456789abcdef"
		var b strings.Builder
		lengths := []int{8, 4, 4, 4, 12}
		for i, l := range lengths {
			if i > 0 {
				b.WriteByte('-')
			}
			for j := 0; j < l; j++ {
				idx := rapid.IntRange(0, len(hexChars)-1).Draw(t, "hexIdx")
				b.WriteByte(hexChars[idx])
			}
		}
		return b.String()
	})
}

// --- Fake client for property tests ---

// historyPropFakeClient tracks whether GetMonitorHistory was called and returns
// configured points.
type historyPropFakeClient struct {
	fakePulseClient
	historyCalled bool
	pointsToReturn []pulseapi.HistoryPoint
}

func (f *historyPropFakeClient) GetMonitorHistory(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
	f.historyCalled = true
	return pulseapi.History{
		MonitorID: id,
		From:      r.From,
		To:        r.To,
		Truncated: false,
		Points:    f.pointsToReturn,
	}, nil
}

// --- Property 11: History returns exactly the points within the range ---
// **Validates: Requirements 6.1, 6.7**
//
// For any history response from Pulse, the output contains all points from the
// response unaltered (state, latency_ms, status_code, error, checked_at).
func TestProperty11_HistoryReturnsExactlyThePointsWithinTheRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a valid time range.
		timeRange := genValidTimeRange().Draw(t, "timeRange")
		from, to := timeRange[0], timeRange[1]

		// Generate 0-20 history points within the range.
		pointCount := rapid.IntRange(0, 20).Draw(t, "pointCount")
		points := make([]pulseapi.HistoryPoint, pointCount)
		for i := 0; i < pointCount; i++ {
			points[i] = genHistoryPoint(from, to).Draw(t, "point")
		}

		monitorID := genValidUUID().Draw(t, "monitorID")

		client := &historyPropFakeClient{
			pointsToReturn: points,
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		out, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
			Monitor: monitorID,
			From:    from.Format(time.RFC3339),
			To:      to.Format(time.RFC3339),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// The output must contain exactly the same number of points.
		if len(out.Points) != len(points) {
			t.Fatalf("expected %d points, got %d", len(points), len(out.Points))
		}

		// Each output point must match its corresponding input point unaltered.
		for i, got := range out.Points {
			src := points[i]

			// State must match exactly.
			if got.State != src.State {
				t.Fatalf("point[%d]: expected state %q, got %q", i, src.State, got.State)
			}

			// LatencyMs must match.
			if (got.LatencyMs == nil) != (src.LatencyMs == nil) {
				t.Fatalf("point[%d]: latency_ms nil mismatch: src=%v, got=%v", i, src.LatencyMs, got.LatencyMs)
			}
			if got.LatencyMs != nil && *got.LatencyMs != *src.LatencyMs {
				t.Fatalf("point[%d]: expected latency_ms=%d, got %d", i, *src.LatencyMs, *got.LatencyMs)
			}

			// StatusCode must match.
			if (got.StatusCode == nil) != (src.StatusCode == nil) {
				t.Fatalf("point[%d]: status_code nil mismatch: src=%v, got=%v", i, src.StatusCode, got.StatusCode)
			}
			if got.StatusCode != nil && *got.StatusCode != *src.StatusCode {
				t.Fatalf("point[%d]: expected status_code=%d, got %d", i, *src.StatusCode, *got.StatusCode)
			}

			// Error must match.
			if (got.Error == nil) != (src.Error == nil) {
				t.Fatalf("point[%d]: error nil mismatch: src=%v, got=%v", i, src.Error, got.Error)
			}
			if got.Error != nil && *got.Error != *src.Error {
				t.Fatalf("point[%d]: expected error=%q, got %q", i, *src.Error, *got.Error)
			}

			// CheckedAt must match (formatted as RFC3339 of the UTC time).
			expectedCheckedAt := src.CheckedAt.UTC().Format(time.RFC3339)
			if got.CheckedAt != expectedCheckedAt {
				t.Fatalf("point[%d]: expected checked_at=%q, got %q", i, expectedCheckedAt, got.CheckedAt)
			}
		}
	})
}

// --- Property 12: from > to is rejected without calling Pulse ---
// **Validates: Requirements 6.4**
//
// For any from timestamp that is strictly after to, the handler returns
// INVALID_RANGE without calling GetMonitorHistory.
func TestProperty12_FromAfterToIsRejectedWithoutCallingPulse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate an inverted time range (from > to).
		timeRange := genInvertedTimeRange().Draw(t, "invertedRange")
		from, to := timeRange[0], timeRange[1]

		monitorID := genValidUUID().Draw(t, "monitorID")

		client := &historyPropFakeClient{}
		deps := Deps{Client: client, AccessMode: config.ReadOnly}

		_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
			Monitor: monitorID,
			From:    from.Format(time.RFC3339),
			To:      to.Format(time.RFC3339),
		})

		// Must return an error.
		if err == nil {
			t.Fatal("expected INVALID_RANGE error when from > to, got nil")
		}

		// Must be an MCPError with INVALID_RANGE code.
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected *mcperr.MCPError, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeInvalidRange {
			t.Fatalf("expected code %q, got %q", mcperr.CodeInvalidRange, mcpErr.Code)
		}

		// GetMonitorHistory must NOT have been called.
		if client.historyCalled {
			t.Fatal("GetMonitorHistory should NOT be called when from > to")
		}
	})
}

// --- Property 13: Malformed monitor identifier is rejected ---
// **Validates: Requirements 6.6**
//
// For any empty or whitespace-only monitor identifier, the handler returns
// INVALID_IDENTIFIER without calling any Pulse method.
func TestProperty13_MalformedMonitorIdentifierIsRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate an empty or whitespace-only identifier.
		malformedID := genWhitespaceOnly().Draw(t, "malformedID")

		client := &historyPropFakeClient{}
		deps := Deps{Client: client, AccessMode: config.ReadOnly}

		_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
			Monitor: malformedID,
		})

		// Must return an error.
		if err == nil {
			t.Fatalf("expected INVALID_IDENTIFIER error for malformed identifier %q, got nil", malformedID)
		}

		// Must be an MCPError with INVALID_IDENTIFIER code.
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected *mcperr.MCPError, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeInvalidIdentifier {
			t.Fatalf("expected code %q, got %q", mcperr.CodeInvalidIdentifier, mcpErr.Code)
		}

		// No Pulse methods should have been called.
		if client.historyCalled {
			t.Fatal("GetMonitorHistory should NOT be called for malformed identifier")
		}
	})
}
