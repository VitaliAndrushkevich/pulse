package notification

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

func TestTruncateErrorDetail_NilForEmpty(t *testing.T) {
	result := TruncateErrorDetail("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %q", *result)
	}
}

func TestTruncateErrorDetail_ShortString(t *testing.T) {
	input := "connection refused"
	result := TruncateErrorDetail(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result != input {
		t.Errorf("expected %q, got %q", input, *result)
	}
}

func TestTruncateErrorDetail_ExactlyMaxLength(t *testing.T) {
	input := strings.Repeat("x", maxErrorDetailLength)
	result := TruncateErrorDetail(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(*result) != maxErrorDetailLength {
		t.Errorf("expected length %d, got %d", maxErrorDetailLength, len(*result))
	}
	if *result != input {
		t.Errorf("expected string unchanged at exact max length")
	}
}

func TestTruncateErrorDetail_TruncatesLongString(t *testing.T) {
	input := strings.Repeat("a", maxErrorDetailLength+500)
	result := TruncateErrorDetail(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(*result) != maxErrorDetailLength {
		t.Errorf("expected length %d, got %d", maxErrorDetailLength, len(*result))
	}
}

func TestTruncateErrorDetail_VeryLongString(t *testing.T) {
	// 10x the max length — must still truncate correctly.
	input := strings.Repeat("Z", maxErrorDetailLength*10)
	result := TruncateErrorDetail(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(*result) != maxErrorDetailLength {
		t.Errorf("expected length %d, got %d", maxErrorDetailLength, len(*result))
	}
}

func TestTruncateErrorDetail_PreservesContent(t *testing.T) {
	// Build a string that is longer than max, verify truncation preserves prefix.
	prefix := "ERROR: something went wrong - "
	padding := strings.Repeat("detail ", 200)
	input := prefix + padding
	if len(input) <= maxErrorDetailLength {
		t.Skip("input not long enough to test truncation")
	}

	result := TruncateErrorDetail(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.HasPrefix(*result, prefix) {
		t.Errorf("expected truncated string to start with %q", prefix)
	}
}

func TestMaxErrorDetailLength_Is1024(t *testing.T) {
	if maxErrorDetailLength != 1024 {
		t.Errorf("maxErrorDetailLength should be 1024, got %d", maxErrorDetailLength)
	}
}

// TestLogDelivery_WithNilQueries verifies LogDelivery doesn't panic when
// queries is nil (it just logs the error internally).
func TestLogDelivery_WithNilQueries(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()
	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 10}, nil, nil, m, st, nil)

	// Should not panic even though queries is nil.
	// LogDelivery will try to call InsertDeliveryLog and get a nil pointer,
	// but this exercises the code path — in production, queries is never nil.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LogDelivery panicked with nil queries: %v", r)
		}
	}()

	d.LogDelivery(context.Background(), uuid.New(), uuid.New(), uuid.Nil,
		"monitor_down", 1, "success", "")
}

// TestProcessJob_PanicRecovery verifies that a panicking dispatch still results
// in the worker continuing to process subsequent jobs.
func TestProcessJob_PanicRecovery(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	st := NewStateTracker()
	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 10}, nil, nil, m, st, nil)

	// Override dispatch to panic on the first call.
	var mu sync.Mutex
	callCount := 0
	d.dispatchFn = func(job DeliveryJob) error {
		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()
		if current == 1 {
			panic("simulated panic in delivery")
		}
		return nil
	}

	d.Start()

	// First job will panic.
	d.Enqueue(DeliveryJob{
		ID:          uuid.New(),
		ChannelID:   uuid.New(),
		MonitorID:   uuid.New(),
		BindingID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
	})

	// Second job should still be processed (worker continues after panic recovery).
	d.Enqueue(DeliveryJob{
		ID:          uuid.New(),
		ChannelID:   uuid.New(),
		MonitorID:   uuid.New(),
		BindingID:   uuid.New(),
		TriggerType: "monitor_up",
		Attempt:     1,
	})

	// Wait for both jobs to be processed.
	deadline := time.After(2 * time.Second)
	for {
		if len(d.jobs) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("jobs not processed within timeout")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Give workers a moment to finish processing.
	time.Sleep(50 * time.Millisecond)
	d.Stop()

	// Verify both jobs were handled.
	mu.Lock()
	if callCount < 2 {
		t.Errorf("expected at least 2 dispatch calls, got %d", callCount)
	}
	mu.Unlock()
}
