// Date formatting and secret reference formatting utilities

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
 * Formats a secret reference for display.
 * Produces the format: "Secret: {name} ({uuid})"
 * No secret value content is ever included.
 */
export function formatSecretReference(name: string, uuid: string): string {
  return `Secret: ${name} (${uuid})`;
}
