package retention

import (
	"fmt"
	"time"
)

const (
	// DefaultCheckInterval is the default retention cleanup interval.
	DefaultCheckInterval = 1 * time.Hour

	// MinCheckInterval is the minimum allowed retention cleanup interval.
	MinCheckInterval = 1 * time.Minute

	// MaxCheckInterval is the maximum allowed retention cleanup interval (7 days).
	MaxCheckInterval = 168 * time.Hour
)

// ParseRetentionInterval parses a Go duration string for the retention check interval.
// If envValue is empty, it returns the default interval (1h).
// If the value is a valid Go duration within [1m, 168h], it returns the parsed duration.
// Otherwise, it returns an error with a descriptive message.
func ParseRetentionInterval(envValue string) (time.Duration, error) {
	if envValue == "" {
		return DefaultCheckInterval, nil
	}

	d, err := time.ParseDuration(envValue)
	if err != nil {
		return 0, fmt.Errorf("invalid PULSE_RETENTION_CHECK_INTERVAL %q: %w", envValue, err)
	}

	if d < MinCheckInterval {
		return 0, fmt.Errorf("PULSE_RETENTION_CHECK_INTERVAL %q is below minimum %s", envValue, MinCheckInterval)
	}

	if d > MaxCheckInterval {
		return 0, fmt.Errorf("PULSE_RETENTION_CHECK_INTERVAL %q exceeds maximum %s", envValue, MaxCheckInterval)
	}

	return d, nil
}
