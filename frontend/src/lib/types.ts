// Core TypeScript types — matches backend OpenAPI contract

export type MonitorType = 'http' | 'http3' | 'tcp' | 'udp' | 'websocket' | 'grpc' | 'dns' | 'icmp' | 'smtp';

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

/** DNS record types supported by the DNS checker */
export type DnsRecordType = 'A' | 'AAAA' | 'CNAME' | 'MX' | 'TXT' | 'SRV' | 'SOA' | 'PTR' | 'NS';

/** DNS monitor settings — matches backend OpenAPI contract */
export interface DnsSettings {
  record_type: DnsRecordType;
  expected_value?: string;
  dns_server?: string;
}

/** ICMP monitor settings — matches backend OpenAPI contract */
export interface IcmpSettings {
  packet_count?: number;
  loss_threshold_percent?: number;
  use_ipv6?: boolean;
}

/** SMTP monitor settings — matches backend OpenAPI contract */
export interface SmtpSettings {
  port?: number;
  starttls?: boolean;
  ehlo_domain?: string;
  ssl_expiry_threshold?: number;
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

// ─── Dashboard Summary Types ────────────────────────────────────────────────

/** Dashboard summary response from GET /api/v1/dashboard/summary */
export interface DashboardSummary {
  health_score: HealthScoreData;
  status_distribution: StatusDistribution;
  active_incidents: ActiveIncident[];
  top_latency_monitors: TopLatencyMonitor[];
  ssl_expiry: SSLExpiryEntry[];
  heatmap: HeatmapHour[];
  recent_events: RecentEvent[];
  generated_at: string;
}

export interface HealthScoreData {
  uptime_percent: number;
  active_monitor_count: number;
  partial_data: boolean;
}

export interface StatusDistribution {
  up: number;
  down: number;
  unknown: number;
  total: number;
}

export interface ActiveIncident {
  monitor_id: string;
  monitor_name: string;
  started_at: string;
  cause: string | null;
  state: 'down';
}

export interface TopLatencyMonitor {
  monitor_id: string;
  monitor_name: string;
  avg_latency_ms: number;
}

export interface SSLExpiryEntry {
  monitor_id: string;
  monitor_name: string;
  days_remaining: number;
  expires_at: string;
}

export interface HeatmapHour {
  hour_start: string;
  up_count: number;
  down_count: number;
  unknown_count: number;
}

export interface RecentEvent {
  monitor_id: string;
  monitor_name: string;
  from_state: 'up' | 'down' | 'unknown';
  to_state: 'up' | 'down' | 'unknown';
  occurred_at: string;
}

export type WidgetId =
  | 'health-score'
  | 'status-ring'
  | 'incidents'
  | 'sparklines'
  | 'ssl-expiry'
  | 'heatmap'
  | 'events-feed';
