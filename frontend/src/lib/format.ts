// Date formatting, latency formatting, and secret reference formatting utilities

/**
 * Formats an ISO timestamp string for display.
 * Returns a locale-aware date/time string, or a placeholder if the value is null.
 */
export function formatDate(isoString: string | null, placeholder = 'Not checked yet'): string {
  if (!isoString) {
    return placeholder;
  }
  const date = new Date(isoString);
  if (isNaN(date.getTime())) {
    return placeholder;
  }
  return date.toLocaleString();
}

/**
 * Format a latency value (given in milliseconds) into a human-readable string
 * with the most appropriate unit.
 *
 * Ranges:
 *   < 1 ms         → microseconds (µs)
 *   1–999 ms       → milliseconds (ms)
 *   1 000–59 999 ms → seconds (s)
 *   ≥ 60 000 ms    → minutes (min)
 */
export function formatLatency(ms: number): string {
  if (ms < 1) {
    // Sub-millisecond: show microseconds
    const us = ms * 1000;
    return `${us < 10 ? us.toFixed(1) : Math.round(us)} µs`;
  }
  if (ms < 1000) {
    // Milliseconds: show integer or 1 decimal for < 10
    return `${ms < 10 ? ms.toFixed(1) : Math.round(ms)} ms`;
  }
  if (ms < 60_000) {
    // Seconds
    const sec = ms / 1000;
    return `${sec < 10 ? sec.toFixed(2) : sec.toFixed(1)} s`;
  }
  // Minutes
  const min = ms / 60_000;
  return `${min < 10 ? min.toFixed(1) : Math.round(min)} min`;
}

/**
 * Formats a secret reference for display.
 * Produces the format: "Secret: {name} ({uuid})"
 * No secret value content is ever included.
 */
export function formatSecretReference(name: string, uuid: string): string {
  return `Secret: ${name} (${uuid})`;
}
