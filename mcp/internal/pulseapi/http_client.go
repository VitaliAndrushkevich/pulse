package pulseapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// httpClient is the concrete HTTP-based implementation of PulseClient.
// It talks to the Pulse REST API using Bearer token auth and per-request timeouts.
type httpClient struct {
	baseURL    string
	token      string
	timeout    time.Duration
	httpClient *http.Client
}

// NewHTTPClient creates a PulseClient backed by the Pulse REST API.
// baseURL is the API root (e.g. "http://localhost:8080/api/v1").
// token is the Pulse Bearer API token.
// timeout is the per-request timeout applied via context.WithTimeout.
func NewHTTPClient(baseURL, token string, timeout time.Duration) PulseClient {
	return &httpClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ListMonitors implements PulseClient.
func (c *httpClient) ListMonitors(ctx context.Context, q MonitorQuery) (MonitorPage, error) {
	params := url.Values{}
	if q.Page > 0 {
		params.Set("page", strconv.Itoa(q.Page))
	}
	if q.Limit > 0 {
		params.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Type != "" {
		params.Set("type", q.Type)
	}
	for _, tag := range q.Tags {
		params.Add("tag", tag)
	}

	var envelope listMonitorsEnvelope
	requestID, err := c.doJSON(ctx, http.MethodGet, "/monitors", params, nil, &envelope)
	if err != nil {
		return MonitorPage{}, err
	}

	page := MonitorPage{
		Monitors:   make([]Monitor, 0, len(envelope.Data)),
		Page:       envelope.Page,
		Limit:      envelope.Limit,
		Total:      envelope.Total,
		TotalPages: envelope.TotalPages,
	}
	for _, m := range envelope.Data {
		page.Monitors = append(page.Monitors, m.toModel())
	}
	_ = requestID
	return page, nil
}

// GetMonitor implements PulseClient.
func (c *httpClient) GetMonitor(ctx context.Context, id string) (Monitor, error) {
	var wire wireMonitor
	_, err := c.doJSON(ctx, http.MethodGet, "/monitors/"+url.PathEscape(id), nil, nil, &wire)
	if err != nil {
		return Monitor{}, err
	}
	return wire.toModel(), nil
}

// GetMonitorStats implements PulseClient.
func (c *httpClient) GetMonitorStats(ctx context.Context, id string) (MonitorStats, error) {
	var wire wireMonitorStats
	_, err := c.doJSON(ctx, http.MethodGet, "/monitors/"+url.PathEscape(id)+"/stats", nil, nil, &wire)
	if err != nil {
		return MonitorStats{}, err
	}
	return wire.toModel(), nil
}

// GetMonitorHistory implements PulseClient.
func (c *httpClient) GetMonitorHistory(ctx context.Context, id string, r TimeRange) (History, error) {
	params := url.Values{}
	if !r.From.IsZero() {
		params.Set("from", r.From.Format(time.RFC3339))
	}
	if !r.To.IsZero() {
		params.Set("to", r.To.Format(time.RFC3339))
	}

	var envelope historyEnvelope
	_, err := c.doJSON(ctx, http.MethodGet, "/monitors/"+url.PathEscape(id)+"/history", params, nil, &envelope)
	if err != nil {
		return History{}, err
	}
	return envelope.toModel(id), nil
}

// ListIncidents implements PulseClient.
func (c *httpClient) ListIncidents(ctx context.Context, q IncidentQuery) (IncidentPage, error) {
	var path string
	params := url.Values{}

	if q.MonitorID != "" {
		path = "/monitors/" + url.PathEscape(q.MonitorID) + "/incidents"
	} else {
		path = "/incidents"
		if q.OpenOnly {
			params.Set("status", "open")
		}
	}
	if q.Page > 0 {
		params.Set("page", strconv.Itoa(q.Page))
	}
	if q.Limit > 0 {
		params.Set("limit", strconv.Itoa(q.Limit))
	}

	var envelope listIncidentsEnvelope
	_, err := c.doJSON(ctx, http.MethodGet, path, params, nil, &envelope)
	if err != nil {
		return IncidentPage{}, err
	}

	page := IncidentPage{
		Incidents:  make([]Incident, 0, len(envelope.Data)),
		Page:       envelope.Page,
		Limit:      envelope.Limit,
		Total:      envelope.Total,
		TotalPages: envelope.TotalPages,
	}
	for _, inc := range envelope.Data {
		page.Incidents = append(page.Incidents, inc.toModel())
	}
	return page, nil
}

// CreateMonitor implements PulseClient.
func (c *httpClient) CreateMonitor(ctx context.Context, in CreateMonitorInput) (Monitor, error) {
	body := createMonitorRequest{
		Type:            in.Type,
		Name:            in.Name,
		Target:          in.Target,
		IntervalSeconds: in.IntervalSeconds,
		TimeoutSeconds:  in.TimeoutSeconds,
		Settings:        in.Settings,
	}

	var wire wireMonitor
	_, err := c.doJSON(ctx, http.MethodPost, "/monitors", nil, body, &wire)
	if err != nil {
		return Monitor{}, err
	}
	return wire.toModel(), nil
}

// doJSON performs an HTTP request and decodes the response into dest.
// It handles auth headers, timeout, error envelopes, connectivity errors, and X-Request-ID.
func (c *httpClient) doJSON(ctx context.Context, method, path string, params url.Values, body any, dest any) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	fullURL := c.baseURL + path
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return "", fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, fullURL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", classifyConnectivityError(err)
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("X-Request-ID")

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return requestID, c.parseErrorResponse(resp, requestID)
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return requestID, fmt.Errorf("decode response: %w", err)
		}
	}
	return requestID, nil
}

// parseErrorResponse reads a non-2xx response and returns a *PulseError.
func (c *httpClient) parseErrorResponse(resp *http.Response, requestID string) error {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return &PulseError{
			Code:       codeFromHTTPStatus(resp.StatusCode),
			Message:    http.StatusText(resp.StatusCode),
			RequestID:  requestID,
			HTTPStatus: resp.StatusCode,
		}
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(bodyBytes, &envelope); err == nil && envelope.Error.Code != "" {
		return &PulseError{
			Code:       envelope.Error.Code,
			Message:    envelope.Error.Message,
			RequestID:  requestID,
			HTTPStatus: resp.StatusCode,
		}
	}

	// Body is not a valid error envelope; synthesize from HTTP status.
	return &PulseError{
		Code:       codeFromHTTPStatus(resp.StatusCode),
		Message:    strings.TrimSpace(string(bodyBytes)),
		RequestID:  requestID,
		HTTPStatus: resp.StatusCode,
	}
}

// classifyConnectivityError maps low-level network/timeout errors to *ConnectivityError.
func classifyConnectivityError(err error) error {
	if err == nil {
		return nil
	}

	// Context deadline exceeded (our per-request timeout).
	if errors.Is(err, context.DeadlineExceeded) {
		return &ConnectivityError{Reason: "timeout"}
	}

	// Check for URL errors wrapping net errors.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if errors.Is(urlErr.Err, context.DeadlineExceeded) {
			return &ConnectivityError{Reason: "timeout"}
		}
		if errors.Is(urlErr.Err, context.Canceled) {
			return &ConnectivityError{Reason: "timeout"}
		}
	}

	// Connection refused.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			if isSyscallConnRefused(opErr.Err) {
				return &ConnectivityError{Reason: "connection_refused"}
			}
			return &ConnectivityError{Reason: "dial_error"}
		}
	}

	// DNS lookup failures and other dial errors.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return &ConnectivityError{Reason: "dial_error"}
	}

	// Generic dial error detection from URL error.
	if urlErr != nil {
		if netOpErr, ok := urlErr.Err.(*net.OpError); ok {
			if netOpErr.Op == "dial" {
				if isSyscallConnRefused(netOpErr.Err) {
					return &ConnectivityError{Reason: "connection_refused"}
				}
				return &ConnectivityError{Reason: "dial_error"}
			}
		}
	}

	// Fallback: if it's any kind of network error, treat as dial_error.
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return &ConnectivityError{Reason: "timeout"}
		}
		return &ConnectivityError{Reason: "dial_error"}
	}

	// If we can't classify it, wrap as dial_error.
	return &ConnectivityError{Reason: "dial_error"}
}

// isSyscallConnRefused checks if a syscall error represents connection refused.
func isSyscallConnRefused(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "connection refused")
}

// codeFromHTTPStatus returns a synthetic error code for an HTTP status when
// the Pulse response body is not a valid error envelope.
func codeFromHTTPStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusUnprocessableEntity:
		return "VALIDATION_ERROR"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusInternalServerError:
		return "INTERNAL_ERROR"
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return "SERVICE_UNAVAILABLE"
	default:
		return "HTTP_" + strconv.Itoa(status)
	}
}
