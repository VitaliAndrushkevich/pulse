package retention

import (
	"testing"
	"time"
)

func TestParseRetentionInterval_EmptyReturnsDefault(t *testing.T) {
	d, err := ParseRetentionInterval("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != DefaultCheckInterval {
		t.Errorf("got %v, want %v", d, DefaultCheckInterval)
	}
}

func TestParseRetentionInterval_ValidDurations(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"1m", 1 * time.Minute},
		{"5m", 5 * time.Minute},
		{"30m", 30 * time.Minute},
		{"1h", 1 * time.Hour},
		{"2h30m", 2*time.Hour + 30*time.Minute},
		{"24h", 24 * time.Hour},
		{"168h", 168 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := ParseRetentionInterval(tt.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if d != tt.want {
				t.Errorf("got %v, want %v", d, tt.want)
			}
		})
	}
}

func TestParseRetentionInterval_InvalidFormat(t *testing.T) {
	tests := []string{
		"abc",
		"1",
		"hour",
		"1d",
		"--1h",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseRetentionInterval(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
		})
	}
}

func TestParseRetentionInterval_BelowMinimum(t *testing.T) {
	tests := []string{
		"59s",
		"30s",
		"1s",
		"500ms",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseRetentionInterval(input)
			if err == nil {
				t.Errorf("expected error for %q (below min), got nil", input)
			}
		})
	}
}

func TestParseRetentionInterval_AboveMaximum(t *testing.T) {
	tests := []string{
		"168h1m",
		"169h",
		"200h",
		"8760h",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseRetentionInterval(input)
			if err == nil {
				t.Errorf("expected error for %q (above max), got nil", input)
			}
		})
	}
}
