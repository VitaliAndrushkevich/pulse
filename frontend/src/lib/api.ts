// API client — fetch wrapper with auth, timeout, and error handling

import { getToken, clearToken } from '$lib/stores/auth.svelte';
import { toastStore } from '$lib/stores/toast.svelte';
import type { Monitor, PaginatedList, HistoryPoint, Incident, Secret } from '$lib/types';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ApiError {
  code: string;
  message: string;
}

export interface ErrorEnvelope {
  error: ApiError;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  token: string;
}

export interface CreateMonitorRequest {
  name: string;
  type: string;
  target: string;
  interval_seconds: number;
  timeout_seconds: number;
  status?: 'active' | 'paused';
  settings?: Record<string, unknown>;
}

export interface UpdateMonitorRequest {
  name: string;
  type: string;
  target: string;
  interval_seconds: number;
  timeout_seconds: number;
  status?: 'active' | 'paused';
  settings?: Record<string, unknown>;
}

export interface CreateSecretRequest {
  name: string;
  value: string;
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const BASE_URL = '/api/v1';
const TIMEOUT_MS = 15000;

// ---------------------------------------------------------------------------
// Error classes
// ---------------------------------------------------------------------------

export class ApiRequestError extends Error {
  public readonly statusCode: number;
  public readonly apiError: ApiError | null;
  public readonly requestId: string | null;

  constructor(statusCode: number, apiError: ApiError | null, requestId: string | null) {
    const message = apiError?.message ?? `Request failed with status ${statusCode}`;
    super(message);
    this.name = 'ApiRequestError';
    this.statusCode = statusCode;
    this.apiError = apiError;
    this.requestId = requestId;
  }
}

export class NetworkError extends Error {
  public readonly requestId: string | null;

  constructor(message: string, requestId: string | null = null) {
    super(message);
    this.name = 'NetworkError';
    this.requestId = requestId;
  }
}

// ---------------------------------------------------------------------------
// Unauthorized handler
// ---------------------------------------------------------------------------

function onUnauthorized(): void {
  clearToken();
  if (typeof window !== 'undefined') {
    window.location.href = '/login';
  }
}

// ---------------------------------------------------------------------------
// Core fetch wrapper
// ---------------------------------------------------------------------------

export interface RequestOptions {
  /** Skip the global 401 handler (used by login page to show inline errors) */
  skipUnauthorizedHandler?: boolean;
  /** Skip showing toast on error (caller handles UI feedback) */
  skipToast?: boolean;
}

/**
 * Generic API request function with:
 * - Bearer token injection from auth store
 * - 15s AbortController timeout
 * - Error envelope parsing
 * - X-Request-ID extraction
 * - 401 handling (clear JWT, redirect to login)
 */
export async function apiRequest<T>(
  method: string,
  path: string,
  body?: unknown,
  options?: RequestOptions
): Promise<T> {
  const url = `${BASE_URL}${path}`;

  // Set up abort controller for 15s timeout
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);

  // Build headers
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  const token = getToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  // Build fetch options
  const fetchOptions: RequestInit = {
    method,
    headers,
    signal: controller.signal,
  };

  if (body !== undefined && method !== 'GET' && method !== 'HEAD') {
    fetchOptions.body = JSON.stringify(body);
  }

  let response: Response;

  try {
    response = await fetch(url, fetchOptions);
  } catch (err: unknown) {
    clearTimeout(timeoutId);

    const isAbort = err instanceof DOMException && err.name === 'AbortError';
    const message = isAbort
      ? 'Request timed out. Please check your connection and try again.'
      : 'Unable to connect to the server. Please check your network connection.';

    // Persistent toast for network errors/timeouts
    if (!options?.skipToast) {
      toastStore.addToast({
        type: 'error',
        message,
        persistent: true,
      });
    }

    throw new NetworkError(message);
  } finally {
    clearTimeout(timeoutId);
  }

  // Extract X-Request-ID
  const requestId = response.headers.get('X-Request-ID');

  // Handle 401 — clear JWT, redirect, suppress toast (unless skipped for login page)
  if (response.status === 401) {
    if (!options?.skipUnauthorizedHandler) {
      onUnauthorized();
    }
    throw new ApiRequestError(401, { code: 'UNAUTHORIZED', message: 'Authentication required' }, requestId);
  }

  // Handle non-2xx responses
  if (!response.ok) {
    let apiError: ApiError | null = null;

    try {
      const json = await response.json();
      if (json?.error?.code && json?.error?.message) {
        apiError = json.error as ApiError;
      }
    } catch {
      // Response body isn't valid JSON or doesn't match envelope
    }

    if (apiError) {
      // Valid error envelope — show message + request ID
      if (!options?.skipToast) {
        toastStore.addToast({
          type: 'error',
          message: apiError.message,
          requestId: requestId ?? undefined,
          persistent: false,
        });
      }
    } else {
      // Non-conforming response — show generic error + request ID
      if (!options?.skipToast) {
        toastStore.addToast({
          type: 'error',
          message: 'An unexpected error occurred. Please try again.',
          requestId: requestId ?? undefined,
          persistent: false,
        });
      }
    }

    throw new ApiRequestError(response.status, apiError, requestId);
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as unknown as T;
  }

  // Parse successful response
  const data = await response.json();
  return data as T;
}

// ---------------------------------------------------------------------------
// Typed endpoint functions
// ---------------------------------------------------------------------------

/** POST /api/v1/auth/login — skips global 401 handler and toast (login page handles errors inline) */
export async function login(credentials: LoginRequest): Promise<LoginResponse> {
  return apiRequest<LoginResponse>('POST', '/auth/login', credentials, {
    skipUnauthorizedHandler: true,
    skipToast: true,
  });
}

/** GET /api/v1/auth/setup — check if initial setup is required */
export async function getSetupStatus(): Promise<{ setup_required: boolean }> {
  return apiRequest<{ setup_required: boolean }>('GET', '/auth/setup', undefined, {
    skipUnauthorizedHandler: true,
    skipToast: true,
  });
}

/** POST /api/v1/auth/setup — create initial admin user */
export async function setupAdmin(credentials: LoginRequest): Promise<LoginResponse> {
  return apiRequest<LoginResponse>('POST', '/auth/setup', credentials, {
    skipUnauthorizedHandler: true,
    skipToast: true,
  });
}

/** GET /api/v1/monitors?page=&limit= */
export async function getMonitors(
  page: number = 1,
  limit: number = 20
): Promise<PaginatedList<Monitor>> {
  return apiRequest<PaginatedList<Monitor>>('GET', `/monitors?page=${page}&limit=${limit}`);
}

/** GET /api/v1/monitors/:id */
export async function getMonitor(id: string): Promise<Monitor> {
  return apiRequest<Monitor>('GET', `/monitors/${id}`);
}

/** POST /api/v1/monitors */
export async function createMonitor(data: CreateMonitorRequest): Promise<Monitor> {
  return apiRequest<Monitor>('POST', '/monitors', data);
}

/** PUT /api/v1/monitors/:id */
export async function updateMonitor(id: string, data: UpdateMonitorRequest): Promise<Monitor> {
  return apiRequest<Monitor>('PUT', `/monitors/${id}`, data);
}

/** DELETE /api/v1/monitors/:id */
export async function deleteMonitor(id: string): Promise<void> {
  return apiRequest<void>('DELETE', `/monitors/${id}`);
}

/** GET /api/v1/monitors/:id/history?from=&to= */
export async function getMonitorHistory(
  id: string,
  from: string,
  to: string
): Promise<HistoryPoint[]> {
  const envelope = await apiRequest<{ monitor_id: string; from: string; to: string; points: HistoryPoint[] }>(
    'GET',
    `/monitors/${id}/history?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  );
  return envelope.points ?? [];
}

/** GET /api/v1/monitors/:id/incidents?page=&limit= */
export async function getMonitorIncidents(
  id: string,
  page: number = 1,
  limit: number = 20
): Promise<PaginatedList<Incident>> {
  return apiRequest<PaginatedList<Incident>>(
    'GET',
    `/monitors/${id}/incidents?page=${page}&limit=${limit}`
  );
}

/** GET /api/v1/secrets */
export async function getSecrets(): Promise<Secret[]> {
  return apiRequest<Secret[]>('GET', '/secrets');
}

/** POST /api/v1/secrets */
export async function createSecret(data: CreateSecretRequest): Promise<Secret> {
  return apiRequest<Secret>('POST', '/secrets', data, { skipToast: true });
}
