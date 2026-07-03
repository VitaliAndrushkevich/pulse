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
  payload_format?: 'raw' | 'proto_json';
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
  implicit_tls?: boolean;
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

// ─── Notification Channel Types ─────────────────────────────────────────────

/** Notification channel type */
export type NotificationChannelType = 'email' | 'webhook';

/** HTTP methods allowed for webhook channels */
export type WebhookMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

/** Custom header for webhook channels */
export interface WebhookHeader {
  name: string;
  value: string;
}

/** Email channel configuration */
export interface EmailChannelConfig {
  recipients: string[];
}

/** Webhook channel configuration */
export interface WebhookChannelConfig {
  url: string;
  method: WebhookMethod;
  body_template: string;
  headers?: WebhookHeader[];
}

/** Notification channel — from channel CRUD API */
export interface NotificationChannel {
  id: string;
  name: string;
  type: NotificationChannelType;
  config: EmailChannelConfig | WebhookChannelConfig;
  created_at: string;
  updated_at: string;
}

/** Trigger condition types */
export type TriggerType = 'monitor_down' | 'monitor_up' | 'degraded' | 'ssl_expiring' | 'n_failures_in_row';

/** A single trigger condition in a binding */
export interface TriggerCondition {
  type: TriggerType;
  threshold_ms?: number;
  days_before?: number;
  count?: number;
}

/** Channel binding — links a channel to a monitor with triggers */
export interface ChannelBinding {
  id: string;
  channel_id: string;
  monitor_id: string;
  triggers: TriggerCondition[];
  reminder_interval_minutes: number | null;
  created_at: string;
  updated_at: string;
}

/** Template variable reference — returned by template-variables endpoint */
export interface TemplateVariable {
  name: string;
  type: string;
  description: string;
  example: string;
}

/** Template variable group */
export interface TemplateVariableGroup {
  name: string;
  variables: TemplateVariable[];
}

/** SMTP settings — returned by GET /notifications/smtp-settings */
export interface SMTPSettings {
  host: string;
  port: number;
  username: string;
  from_address: string;
  tls_enabled: boolean;
  password_set: boolean;
}

/** SMTP settings update request */
export interface SMTPSettingsRequest {
  host: string;
  port: number;
  username?: string;
  password?: string;
  from_address: string;
  tls_enabled: boolean;
}

/** Test channel result */
export interface TestChannelResult {
  success: boolean;
  channel_type: NotificationChannelType;
  channel_id: string;
  error?: string;
}

/** Test SMTP result */
export interface TestSMTPResult {
  success: boolean;
  error?: string;
}

// ─── Proto Source Types ─────────────────────────────────────────────────────

/** Proto source metadata returned by the proto-source API */
export interface ProtoSourceMeta {
  source_type: 'upload' | 'reflection';
  filenames: string[];
  services: ProtoService[];
  created_at: string;
  size_bytes: number;
}

/** A gRPC service discovered from a proto source */
export interface ProtoService {
  full_name: string;
  methods: ProtoMethod[];
}

/** A single RPC method within a proto service */
export interface ProtoMethod {
  name: string;
  full_name: string;
  input_type: string;
  output_type: string;
}

/** Selection result from the service/method selector */
export interface ServiceMethodSelection {
  service_name: string;   // e.g., "mypackage.MyService"
  method_name: string;    // e.g., "GetItem"
  full_method: string;    // e.g., "mypackage.MyService/GetItem"
  input_type: string;     // e.g., "mypackage.GetItemRequest"
  output_type: string;    // e.g., "mypackage.GetItemResponse"
}

/** Schema for a protobuf message type */
export interface ProtoMessageSchema {
  full_name: string;
  fields: ProtoField[];
}

/** A single field in a protobuf message definition */
export interface ProtoField {
  name: string;
  json_name: string;
  type: string;
  repeated: boolean;
  map_key_type?: string;
  map_value_type?: string;
  enum_values?: string[];
  message_fields?: ProtoField[];
  comment?: string;
}
