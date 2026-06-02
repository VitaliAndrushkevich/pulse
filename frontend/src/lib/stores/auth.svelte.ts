/**
 * AuthStore — JWT session management using Svelte 5 runes.
 *
 * Singleton module-level store that reads/writes JWT from localStorage.
 * Handles SSR gracefully by checking for window/localStorage availability.
 *
 * This file uses .svelte.ts extension to enable Svelte 5 rune transforms
 * at module level ($state, $derived).
 *
 * Requirements: 6.1, 6.2, 6.5, 6.7, 6.9
 */

const STORAGE_KEY = 'pulse_jwt';

function readTokenFromStorage(): string | null {
  if (typeof window === 'undefined') return null;
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    // localStorage may be unavailable (e.g. private browsing in some browsers)
    return null;
  }
}

function writeTokenToStorage(token: string): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, token);
  } catch {
    // Silently fail — storage quota or access issue
  }
}

function removeTokenFromStorage(): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Silently fail
  }
}

// --- Reactive state (Svelte 5 runes) ---

let token = $state<string | null>(readTokenFromStorage());
const authenticated = $derived(token !== null);

/**
 * Whether the user has an active session (JWT is present).
 * Exported as a getter function per Svelte 5 module rules
 * (cannot export $derived directly).
 */
export function isAuthenticated(): boolean {
  return authenticated;
}

/**
 * Return the current JWT token value.
 * Used by API client for Bearer header and WS client for query param.
 */
export function getToken(): string | null {
  return token;
}

/**
 * Store a new JWT token in state and localStorage.
 * Called after successful login.
 */
export function setToken(newToken: string): void {
  token = newToken;
  writeTokenToStorage(newToken);
}

/**
 * Clear the JWT from state and localStorage.
 * Called on logout or when a 401 is received.
 */
export function clearToken(): void {
  token = null;
  removeTokenFromStorage();
}
