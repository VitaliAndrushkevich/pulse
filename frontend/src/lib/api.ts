// API client — fetch wrapper with auth, timeout, and error handling

import { getToken, clearToken } from '$lib/stores/auth.svelte';
import { toastStore } from '$lib/stores/toast.svelte';
import type {
  Monitor, PaginatedList, HistoryPoint, Incident, Secret, Tag, DashboardSummary, ProtoSourceMeta,
  NotificationChannel, NotificationChannelType, ChannelBinding, TriggerCondition,
  TemplateVariableGroup, SMTPSettings, SMTPSettingsRequest, TestChannelResult, TestSMTPResult,
  EmailChannelConfig, WebhookChannelConfig
} from '$lib/types';

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
  tags?: Tag[];
  history_retention_days?: number;
}

export interface UpdateMonitorRequest {
  name: string;
  type: string;
  target: string;
  interval_seconds: number;
  timeout_seconds: number;
  status?: 'active' | 'paused';
  settings?: Record<string, unknown>;
  tags?: Tag[];
  history_retention_days?: number;
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

/** GET /api/v1/monitors?page=&limit=&type=&tag=key:value */
export async function getMonitors(
  page: number = 1,
  limit: number = 20,
  options?: { type?: string; tags?: string[] }
): Promise<PaginatedList<Monitor>> {
  const params = new URLSearchParams();
  params.set('page', String(page));
  params.set('limit', String(limit));

  if (options?.type) {
    params.set('type', options.type);
  }

  if (options?.tags) {
    for (const tag of options.tags) {
      params.append('tag', tag);
    }
  }

  return apiRequest<PaginatedList<Monitor>>('GET', `/monitors?${params.toString()}`);
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

/** GET /api/v1/tags — list all distinct tag keys */
export async function getTags(): Promise<string[]> {
  const envelope = await apiRequest<{ data: string[] }>('GET', '/tags');
  return envelope.data;
}

/** GET /api/v1/tags/:key — list distinct values for a tag key */
export async function getTagValues(key: string): Promise<string[]> {
  const envelope = await apiRequest<{ data: string[] }>('GET', `/tags/${encodeURIComponent(key)}`);
  return envelope.data;
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

// ---------------------------------------------------------------------------
// Extended History (aggregated/downsampled)
// ---------------------------------------------------------------------------

/** Aggregated history point — returned when step is used */
export interface AggregatedHistoryPoint {
  timestamp: string;
  min_latency_ms: number | null;
  max_latency_ms: number | null;
  avg_latency_ms: number | null;
  check_count: number;
  uptime_ratio: number;
}

/** Extended history response — supports both raw and aggregated points */
export interface HistoryResponseExtended {
  monitor_id: string;
  from: string;
  to: string;
  points?: HistoryPoint[];
  aggregated_points?: AggregatedHistoryPoint[];
  step?: number;
  truncated?: boolean;
}

/** GET /api/v1/monitors/:id/history?from=&to=&step= (extended response with aggregation support) */
export async function getMonitorHistoryExtended(
  id: string,
  from: string,
  to: string,
  step?: number
): Promise<HistoryResponseExtended> {
  const params = new URLSearchParams();
  params.set('from', from);
  params.set('to', to);
  if (step !== undefined) {
    params.set('step', String(step));
  }
  return apiRequest<HistoryResponseExtended>(
    'GET',
    `/monitors/${id}/history?${params.toString()}`
  );
}

/** GET /api/v1/monitors/:id/stats — uptime percentages, SSL info, last error */
export async function getMonitorStats(id: string): Promise<import('$lib/types').MonitorStats> {
  return apiRequest<import('$lib/types').MonitorStats>('GET', `/monitors/${id}/stats`);
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
export async function getSecrets(
  page: number = 1,
  limit: number = 100
): Promise<PaginatedList<Secret>> {
  return apiRequest<PaginatedList<Secret>>('GET', `/secrets?page=${page}&limit=${limit}`);
}

/** POST /api/v1/secrets */
export async function createSecret(data: CreateSecretRequest): Promise<Secret> {
  return apiRequest<Secret>('POST', '/secrets', data, { skipToast: true });
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

/** GET /api/v1/dashboard/summary — aggregated health overview for all widgets */
export async function getDashboardSummary(): Promise<DashboardSummary> {
  return apiRequest<DashboardSummary>('GET', '/dashboard/summary');
}

// ---------------------------------------------------------------------------
// Monitor Credentials
// ---------------------------------------------------------------------------

export interface Credential {
  id: string;
  auth_type: 'bearer' | 'basic' | 'header';
  name: string;
  header_name?: string;
  username?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateCredentialRequest {
  auth_type: 'bearer' | 'basic' | 'header';
  name: string;
  token?: string;
  username?: string;
  password?: string;
  header_name?: string;
  header_value?: string;
}

/** POST /api/v1/monitors/:id/credentials */
export async function createCredential(
  monitorId: string,
  req: CreateCredentialRequest
): Promise<Credential> {
  return apiRequest<Credential>('POST', `/monitors/${monitorId}/credentials`, req);
}

/** GET /api/v1/monitors/:id/credentials */
export async function listCredentials(monitorId: string): Promise<Credential[]> {
  return apiRequest<Credential[]>('GET', `/monitors/${monitorId}/credentials`);
}

/** PUT /api/v1/monitors/:id/credentials/:credentialId */
export async function updateCredential(
  monitorId: string,
  credId: string,
  req: Partial<CreateCredentialRequest>
): Promise<Credential> {
  return apiRequest<Credential>('PUT', `/monitors/${monitorId}/credentials/${credId}`, req);
}

/** DELETE /api/v1/monitors/:id/credentials/:credentialId */
export async function deleteCredential(monitorId: string, credId: string): Promise<void> {
  return apiRequest<void>('DELETE', `/monitors/${monitorId}/credentials/${credId}`);
}

// ---------------------------------------------------------------------------
// API Tokens
// ---------------------------------------------------------------------------

export interface ApiToken {
  id: string;
  name: string;
  last_used_at?: string | null;
  expires_at?: string | null;
  revoked_at?: string | null;
  created_at: string;
}

export interface CreateApiTokenRequest {
  name: string;
  expires_at?: string;
}

export interface CreateApiTokenResponse {
  token: string;
  id: string;
  name: string;
  expires_at?: string | null;
  created_at: string;
}

/** GET /api/v1/tokens?page=&limit= */
export async function listApiTokens(
  page: number = 1,
  limit: number = 100
): Promise<PaginatedList<ApiToken>> {
  return apiRequest<PaginatedList<ApiToken>>('GET', `/tokens?page=${page}&limit=${limit}`);
}

/** POST /api/v1/tokens */
export async function createApiToken(data: CreateApiTokenRequest): Promise<CreateApiTokenResponse> {
  return apiRequest<CreateApiTokenResponse>('POST', '/tokens', data, { skipToast: true });
}

/** DELETE /api/v1/tokens/:id (revoke) */
export async function revokeApiToken(id: string): Promise<ApiToken> {
  return apiRequest<ApiToken>('DELETE', `/tokens/${id}`);
}

// ---------------------------------------------------------------------------
// Proto Source Management
// ---------------------------------------------------------------------------

/**
 * Upload .proto or .desc files for a monitor's proto source.
 * POST /api/v1/monitors/{id}/proto-source (multipart/form-data)
 */
export async function uploadProtoSource(
  monitorId: string,
  files: File[]
): Promise<ProtoSourceMeta> {
  const url = `${BASE_URL}/monitors/${monitorId}/proto-source`;

  const formData = new FormData();
  for (const file of files) {
    formData.append('file', file);
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);

  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  // NOTE: Do NOT set Content-Type — browser will set it with multipart boundary.

  let response: Response;
  try {
    response = await fetch(url, {
      method: 'POST',
      headers,
      body: formData,
      signal: controller.signal,
    });
  } catch (err: unknown) {
    clearTimeout(timeoutId);
    const isAbort = err instanceof DOMException && err.name === 'AbortError';
    const message = isAbort
      ? 'Upload timed out. Please check your connection and try again.'
      : 'Unable to connect to the server. Please check your network connection.';
    throw new NetworkError(message);
  } finally {
    clearTimeout(timeoutId);
  }

  const requestId = response.headers.get('X-Request-ID');

  if (!response.ok) {
    const envelope = (await response.json().catch(() => null)) as ErrorEnvelope | null;
    throw new ApiRequestError(response.status, envelope?.error ?? null, requestId);
  }

  return (await response.json()) as ProtoSourceMeta;
}

/**
 * Trigger Server Reflection discovery for a monitor's proto source.
 * POST /api/v1/monitors/{id}/proto-source/reflect
 */
export async function triggerReflection(monitorId: string): Promise<ProtoSourceMeta> {
  return apiRequest<ProtoSourceMeta>('POST', `/monitors/${monitorId}/proto-source/reflect`);
}

/**
 * Ad-hoc Server Reflection — discover services without a saved monitor.
 * POST /api/v1/grpc/reflect
 * Used during monitor creation when no monitorId exists yet.
 */
export async function adHocReflect(target: string, tlsMode: string = 'tls'): Promise<ProtoSourceMeta> {
  return apiRequest<ProtoSourceMeta>('POST', '/grpc/reflect', { target, tls_mode: tlsMode });
}

/**
 * Ad-hoc proto file parsing — parse .proto/.desc files without a saved monitor.
 * POST /api/v1/grpc/parse-proto (multipart/form-data)
 * Used during monitor creation to discover services before the monitor exists.
 */
export async function adHocParseProto(files: File[]): Promise<ProtoSourceMeta> {
  const url = `${BASE_URL}/grpc/parse-proto`;

  const formData = new FormData();
  for (const file of files) {
    formData.append('file', file);
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);

  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  let response: Response;
  try {
    response = await fetch(url, {
      method: 'POST',
      headers,
      body: formData,
      signal: controller.signal,
    });
  } catch (err: unknown) {
    clearTimeout(timeoutId);
    const isAbort = err instanceof DOMException && err.name === 'AbortError';
    const message = isAbort
      ? 'Upload timed out. Please check your connection and try again.'
      : 'Unable to connect to the server. Please check your network connection.';
    throw new NetworkError(message);
  } finally {
    clearTimeout(timeoutId);
  }

  const requestId = response.headers.get('X-Request-ID');

  if (!response.ok) {
    const envelope = (await response.json().catch(() => null)) as ErrorEnvelope | null;
    throw new ApiRequestError(response.status, envelope?.error ?? null, requestId);
  }

  return (await response.json()) as ProtoSourceMeta;
}

/**
 * Get the current proto source metadata for a monitor.
 * GET /api/v1/monitors/{id}/proto-source
 * Returns null if no proto source is configured (404).
 */
export async function getProtoSource(monitorId: string): Promise<ProtoSourceMeta | null> {
  try {
    return await apiRequest<ProtoSourceMeta>('GET', `/monitors/${monitorId}/proto-source`, undefined, {
      skipToast: true,
    });
  } catch (err) {
    if (err instanceof ApiRequestError && err.statusCode === 404) {
      return null;
    }
    throw err;
  }
}

/**
 * Delete the proto source for a monitor.
 * DELETE /api/v1/monitors/{id}/proto-source
 */
export async function deleteProtoSource(monitorId: string): Promise<void> {
  await apiRequest<{ ok: boolean }>('DELETE', `/monitors/${monitorId}/proto-source`);
}


// ---------------------------------------------------------------------------
// Notification Channels
// ---------------------------------------------------------------------------

export interface CreateNotificationChannelRequest {
  name: string;
  type: NotificationChannelType;
  config: EmailChannelConfig | WebhookChannelConfig;
}

export interface UpdateNotificationChannelRequest {
  name: string;
  type: NotificationChannelType;
  config: EmailChannelConfig | WebhookChannelConfig;
}

/** POST /api/v1/notifications/channels */
export async function createNotificationChannel(
  data: CreateNotificationChannelRequest
): Promise<NotificationChannel> {
  return apiRequest<NotificationChannel>('POST', '/notifications/channels', data);
}

/** GET /api/v1/notifications/channels?page=&limit= */
export async function listNotificationChannels(
  page: number = 1,
  limit: number = 20
): Promise<PaginatedList<NotificationChannel>> {
  return apiRequest<PaginatedList<NotificationChannel>>(
    'GET',
    `/notifications/channels?page=${page}&limit=${limit}`
  );
}

/** GET /api/v1/notifications/channels/:id */
export async function getNotificationChannel(id: string): Promise<NotificationChannel> {
  return apiRequest<NotificationChannel>('GET', `/notifications/channels/${id}`);
}

/** PUT /api/v1/notifications/channels/:id */
export async function updateNotificationChannel(
  id: string,
  data: UpdateNotificationChannelRequest
): Promise<NotificationChannel> {
  return apiRequest<NotificationChannel>('PUT', `/notifications/channels/${id}`, data);
}

/** DELETE /api/v1/notifications/channels/:id */
export async function deleteNotificationChannel(id: string): Promise<void> {
  return apiRequest<void>('DELETE', `/notifications/channels/${id}`);
}

/** POST /api/v1/notifications/channels/:id/test */
export async function testNotificationChannel(id: string): Promise<TestChannelResult> {
  return apiRequest<TestChannelResult>('POST', `/notifications/channels/${id}/test`);
}

/** GET /api/v1/notifications/template-variables */
export async function getTemplateVariables(): Promise<TemplateVariableGroup[]> {
  const res = await apiRequest<{ groups: TemplateVariableGroup[] }>('GET', '/notifications/template-variables');
  return res.groups;
}

// ---------------------------------------------------------------------------
// Notification Bindings (per-monitor)
// ---------------------------------------------------------------------------

export interface CreateBindingRequest {
  channel_id: string;
  triggers: TriggerCondition[];
  reminder_interval_minutes?: number | null;
}

export interface UpdateBindingRequest {
  triggers: TriggerCondition[];
  reminder_interval_minutes?: number | null;
}

/** POST /api/v1/monitors/:id/notification-bindings */
export async function createNotificationBinding(
  monitorId: string,
  data: CreateBindingRequest
): Promise<ChannelBinding> {
  return apiRequest<ChannelBinding>(
    'POST',
    `/monitors/${monitorId}/notification-bindings`,
    data
  );
}

/** GET /api/v1/monitors/:id/notification-bindings */
export async function listNotificationBindings(
  monitorId: string
): Promise<ChannelBinding[]> {
  const result = await apiRequest<PaginatedList<ChannelBinding>>(
    'GET',
    `/monitors/${monitorId}/notification-bindings`
  );
  return result.data;
}

/** PUT /api/v1/monitors/:id/notification-bindings/:bindingId */
export async function updateNotificationBinding(
  monitorId: string,
  bindingId: string,
  data: UpdateBindingRequest
): Promise<ChannelBinding> {
  return apiRequest<ChannelBinding>(
    'PUT',
    `/monitors/${monitorId}/notification-bindings/${bindingId}`,
    data
  );
}

/** DELETE /api/v1/monitors/:id/notification-bindings/:bindingId */
export async function deleteNotificationBinding(
  monitorId: string,
  bindingId: string
): Promise<void> {
  return apiRequest<void>(
    'DELETE',
    `/monitors/${monitorId}/notification-bindings/${bindingId}`
  );
}

// ---------------------------------------------------------------------------
// SMTP Settings
// ---------------------------------------------------------------------------

/** GET /api/v1/notifications/smtp-settings */
export async function getSMTPSettings(): Promise<SMTPSettings | null> {
  try {
    return await apiRequest<SMTPSettings>('GET', '/notifications/smtp-settings', undefined, {
      skipToast: true,
    });
  } catch (err) {
    if (err instanceof ApiRequestError && err.statusCode === 404) {
      return null;
    }
    throw err;
  }
}

/** PUT /api/v1/notifications/smtp-settings */
export async function updateSMTPSettings(data: SMTPSettingsRequest): Promise<SMTPSettings> {
  return apiRequest<SMTPSettings>('PUT', '/notifications/smtp-settings', data);
}

/** DELETE /api/v1/notifications/smtp-settings */
export async function deleteSMTPSettings(): Promise<void> {
  return apiRequest<void>('DELETE', '/notifications/smtp-settings');
}

/** POST /api/v1/notifications/smtp-settings/test */
export async function testSMTPSettings(data?: SMTPSettingsRequest): Promise<TestSMTPResult> {
  return apiRequest<TestSMTPResult>('POST', '/notifications/smtp-settings/test', data);
}
