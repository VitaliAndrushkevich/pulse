package heartbeat

import "time"

// NextDeadline computes the next expected check-in deadline for a heartbeat:
//
//	next = lastSuccess + intervalSeconds + graceSeconds
//
// It is the single source of truth for the invariant
// next_expected_deadline = last_success_at + interval + grace (design §Invariant
// Ownership) and is applied by the write path on each success ping and on a PUT
// that changes interval/grace while a last_success_at exists. The sweep
// evaluator never recomputes it.
//
// This is a pure function with no clock or DB access so it is directly unit
// testable. Callers pass lastSuccess in UTC; the result preserves that location.
func NextDeadline(lastSuccess time.Time, intervalSeconds, graceSeconds int) time.Time {
	return lastSuccess.Add(time.Duration(intervalSeconds+graceSeconds) * time.Second)
}
