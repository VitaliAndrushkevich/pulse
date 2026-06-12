// Core TypeScript types — matches backend OpenAPI contract

export type MonitorType = 'http' | 'http3' | 'tcp' | 'udp' | 'websocket' | 'grpc';

/** TLS connection mode for gRPC monitors */
export type TlsMode = 'plaintext' | 'tls' | 'tls_skip_verify';

/** gRPC monitor settings — matches backend OpenAPI contract */
export interface GrpcSettings {
  service_method: string;
  tls_mode: TlsMode;
  ssl_expiry_threshold?: number;
  metadata?: Record<string, string>;
  expected_statuses: number[];
  request_payload?: string;
}

/** Tag — key-value pair associated with a monitor */
export interface Tag {
  key: string;
  value: string;
}

/** Monitor — matches OpenAPI Monitor schema */
export interface Monitor {
  id: string;
  name: string;
  type: MonitorType;
  target: string;
  interval_seconds: number;
  timeout_seconds: number;
  status: 'active' | 'paused';
  state: 'up' | 'down' | 'unknown';
  last_checked_at: string | null;
  next_check_at: string | null;
  settings: Record<string, unknown>;
  tags: Tag[];
  history_retention_days: number;
  created_at: string;
  updated_at: string;
}

/** Filter state for monitor listing */
export interface MonitorFilters {
  types: MonitorType[];
  tags: Tag[];
  page: number;
  limit: number;
}

/** History point — from GET /monitors/{id}/history */
export interface HistoryPoint {
  state: 'up' | 'down';
  latency_ms: number | null;
  status_code: number | null;
  error: string | null;
  ssl_days_remaining: number | null;
  checked_at: string;
}

/** Incident — from GET /monitors/{id}/incidents */
export interface Incident {
  id: string;
  monitor_id: string;
  started_at: string;
  resolved_at: string | null;
  cause: string | null;
  created_at: string;
}

/** Secret — metadata only, no value */
export interface Secret {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

/** Paginated API response envelope */
export interface PaginatedList<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

/** WebSocket message envelope */
export interface WsEnvelope<T = unknown> {
  type: string;
  payload: T;
}

/** Patch payload from WebSocket monitor_status messages */
export interface MonitorPatch {
  monitor_id: string;
  state: 'up' | 'down' | 'unknown';
  latency_ms: number;
  status_code?: number;
  ssl_days_remaining?: number;
  error?: string;
  checked_at: string;
  timestamp: string;
}

/** Payload from WebSocket monitor_tags_changed messages */
export interface MonitorTagsChangedPayload {
  monitor_id: string;
  tags: Tag[];
  timestamp: string;
}

/** Uptime stats for a time window */
export interface UptimeWindowStats {
  total_checks: number;
  up_checks: number;
  uptime_percent: number;
  avg_latency_ms: number;
}

/** SSL certificate information */
export interface SSLInfo {
  days_remaining: number;
  expires_at: string;
}

/** Last error information */
export interface LastErrorInfo {
  error: string;
  checked_at: string;
}

/** Monitor statistics — from GET /monitors/{id}/stats */
export interface MonitorStats {
  monitor_id: string;
  uptime_24h: UptimeWindowStats;
  uptime_30d: UptimeWindowStats;
  ssl?: SSLInfo;
  last_error?: LastErrorInfo;
}
