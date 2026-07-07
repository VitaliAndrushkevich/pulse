package tools

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// Feature: mcp-server, Property 1: Pagination arithmetic is consistent
//
// For any valid page/limit and total returned by Pulse, the handler computes
// total_pages = ceil(total/limit) and has_next_page = (page < total_pages).
//
// **Validates: Requirements 4.4, 8.4**
func TestProperty1_PaginationArithmeticConsistent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		page := rapid.IntRange(1, 100).Draw(t, "page")
		limit := rapid.IntRange(1, 100).Draw(t, "limit")
		total := rapid.IntRange(0, 10000).Draw(t, "total")

		expectedTotalPages := int(math.Ceil(float64(total) / float64(limit)))

		client := &fakePulseClient{
			listMonitorsFunc: func(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
				return pulseapi.MonitorPage{
					Monitors:   nil,
					Page:       q.Page,
					Limit:      q.Limit,
					Total:      total,
					TotalPages: expectedTotalPages,
				}, nil
			},
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		out, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{
			Page:  page,
			Limit: limit,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify total_pages = ceil(total/limit)
		if out.TotalPages != expectedTotalPages {
			t.Fatalf("total_pages: got %d, want ceil(%d/%d) = %d",
				out.TotalPages, total, limit, expectedTotalPages)
		}

		// Verify has_next_page = (page < total_pages)
		expectedHasNext := page < expectedTotalPages
		if out.HasNextPage != expectedHasNext {
			t.Fatalf("has_next_page: got %v, want %v (page=%d, total_pages=%d)",
				out.HasNextPage, expectedHasNext, page, expectedTotalPages)
		}
	})
}

// Feature: mcp-server, Property 2: Out-of-range page/limit is rejected without calling Pulse
//
// For any page<1 or limit<1 or limit>100, the handler returns an error
// without invoking PulseClient.ListMonitors.
//
// **Validates: Requirements 4.8, 8.7**
func TestProperty2_OutOfRangePageLimitRejectedWithoutCallingPulse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate at least one invalid dimension.
		// We use a selector to decide which dimension(s) to make invalid.
		invalidDim := rapid.IntRange(0, 2).Draw(t, "invalidDim")

		var page, limit int
		switch invalidDim {
		case 0:
			// Invalid page (negative), valid limit
			page = rapid.IntRange(-1000, -1).Draw(t, "page")
			limit = rapid.IntRange(1, 100).Draw(t, "limit")
		case 1:
			// Valid page, limit < 1
			page = rapid.IntRange(1, 100).Draw(t, "page")
			limit = rapid.IntRange(-1000, -1).Draw(t, "limit")
		case 2:
			// Valid page, limit > 100
			page = rapid.IntRange(1, 100).Draw(t, "page")
			limit = rapid.IntRange(101, 10000).Draw(t, "limit")
		}

		called := false
		client := &fakePulseClient{
			listMonitorsFunc: func(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
				called = true
				return pulseapi.MonitorPage{}, nil
			},
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{
			Page:  page,
			Limit: limit,
		})

		if err == nil {
			t.Fatalf("expected error for page=%d, limit=%d but got nil", page, limit)
		}

		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected MCPError, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeInvalidRange {
			t.Fatalf("expected code %q, got %q", mcperr.CodeInvalidRange, mcpErr.Code)
		}

		if called {
			t.Fatal("PulseClient.ListMonitors was called despite invalid page/limit")
		}
	})
}

// Feature: mcp-server, Property 3: Empty match returns an empty set, not an error
//
// When Pulse returns total=0 and an empty monitors slice, the handler returns
// monitors=[], total=0 (not an error).
//
// **Validates: Requirements 4.6, 8.5**
func TestProperty3_EmptyMatchReturnsEmptySetNotError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		page := rapid.IntRange(1, 50).Draw(t, "page")
		limit := rapid.IntRange(1, 100).Draw(t, "limit")

		client := &fakePulseClient{
			listMonitorsFunc: func(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
				return pulseapi.MonitorPage{
					Monitors:   []pulseapi.Monitor{},
					Page:       page,
					Limit:      limit,
					Total:      0,
					TotalPages: 0,
				}, nil
			},
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		out, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{
			Page:  page,
			Limit: limit,
		})

		if err != nil {
			t.Fatalf("expected no error for empty result, got: %v", err)
		}
		if out.Total != 0 {
			t.Fatalf("expected total=0, got %d", out.Total)
		}
		if len(out.Monitors) != 0 {
			t.Fatalf("expected empty monitors slice, got %d items", len(out.Monitors))
		}
		if out.HasNextPage {
			t.Fatal("expected has_next_page=false for empty result")
		}
	})
}

// Feature: mcp-server, Property 4: Monitor-type filter normalizes case-insensitively
//
// For any recognized type string in any case variation, it normalizes to the
// canonical wire form passed to PulseClient.ListMonitors.
//
// **Validates: Requirements 4.2**
func TestProperty4_MonitorTypeFilterNormalizesCaseInsensitively(t *testing.T) {
	// Build a list of (recognized input, canonical wire form) pairs.
	type typeMapping struct {
		input     string
		canonical string
	}
	knownTypes := []typeMapping{
		{"http", "http"},
		{"https", "https"},
		{"http/3", "http3"},
		{"tcp", "tcp"},
		{"udp", "udp"},
		{"websocket", "websocket"},
		{"grpc", "grpc"},
		{"dns", "dns"},
		{"icmp", "icmp"},
		{"smtp", "smtp"},
		{"quic", "quic"},
	}

	rapid.Check(t, func(t *rapid.T) {
		// Pick a random recognized type
		idx := rapid.IntRange(0, len(knownTypes)-1).Draw(t, "typeIdx")
		mapping := knownTypes[idx]

		// Generate a random case variation of the input
		caseVariation := randomCase(t, mapping.input)

		var gotType string
		client := &fakePulseClient{
			listMonitorsFunc: func(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
				gotType = q.Type
				return pulseapi.MonitorPage{Page: 1, Limit: 50, Total: 0, TotalPages: 0}, nil
			},
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{Type: caseVariation})

		if err != nil {
			t.Fatalf("unexpected error for type %q (case variation of %q): %v",
				caseVariation, mapping.input, err)
		}
		if gotType != mapping.canonical {
			t.Fatalf("type %q (variation of %q) normalized to %q, want %q",
				caseVariation, mapping.input, gotType, mapping.canonical)
		}
	})
}

// Feature: mcp-server, Property 5: Unrecognized type filter is rejected without calling Pulse
//
// For any type string NOT in the recognized set, the handler returns an
// INVALID_TYPE error without invoking PulseClient.ListMonitors.
//
// **Validates: Requirements 4.5**
func TestProperty5_UnrecognizedTypeRejectedWithoutCallingPulse(t *testing.T) {
	// Set of recognized types (lowercase) for filtering
	recognized := map[string]bool{
		"http": true, "https": true, "http/3": true,
		"tcp": true, "udp": true, "websocket": true,
		"grpc": true, "dns": true, "icmp": true, "smtp": true,
	}

	rapid.Check(t, func(t *rapid.T) {
		// Generate an arbitrary type string that is NOT in the recognized set
		typeStr := rapid.StringMatching(`[A-Za-z0-9/]{1,20}`).Filter(func(s string) bool {
			return !recognized[strings.ToLower(strings.TrimSpace(s))]
		}).Draw(t, "type")

		called := false
		client := &fakePulseClient{
			listMonitorsFunc: func(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
				called = true
				return pulseapi.MonitorPage{}, nil
			},
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{Type: typeStr})

		if err == nil {
			t.Fatalf("expected error for unrecognized type %q but got nil", typeStr)
		}

		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected MCPError, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeInvalidType {
			t.Fatalf("expected code %q, got %q", mcperr.CodeInvalidType, mcpErr.Code)
		}

		if called {
			t.Fatal("PulseClient.ListMonitors was called despite unrecognized type")
		}
	})
}

// randomCase applies random casing to each character of a string using rapid.
func randomCase(t *rapid.T, s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i, ch := range s {
		if rapid.Bool().Draw(t, strings.Join([]string{"upper", string(rune('0' + i))}, "")) {
			b.WriteString(strings.ToUpper(string(ch)))
		} else {
			b.WriteString(strings.ToLower(string(ch)))
		}
	}
	return b.String()
}
