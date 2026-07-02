package monitor

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/VitaliAndrushkevich/pulse/internal/proto"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// GRPCSettings holds configuration for the gRPC checker.
// These fields are stored in the monitor's `settings` JSON column
// and configured at monitor creation time.
type GRPCSettings struct {
	// ServiceMethod is the fully-qualified service/method in "package.Service/Method" format.
	// Default: "" (falls back to grpc.health.v1.Health/Check).
	ServiceMethod string `json:"service_method,omitempty"`

	// TLSMode controls connection security: "plaintext", "tls", or "tls_skip_verify".
	// Default: "tls".
	TLSMode string `json:"tls_mode,omitempty"`

	// SSLExpiryThreshold is the minimum acceptable days until certificate expiry.
	// If the cert expires within this many days, the check reports "down".
	// Range: 1–3650. Default: 0 (disabled).
	SSLExpiryThreshold int `json:"ssl_expiry_threshold,omitempty"`

	// Metadata are key-value pairs sent as gRPC request metadata.
	// Max 20 entries. Keys: lowercase alphanumeric, hyphen, underscore, dot.
	// Keys must not start with "grpc-". Values max 4096 chars.
	Metadata map[string]string `json:"metadata,omitempty"`

	// ExpectedStatuses is a list of gRPC status codes considered "up".
	// Values in range 0–16. Default: [0] (OK only).
	ExpectedStatuses []int `json:"expected_statuses,omitempty"`

	// RequestPayload is a base64-encoded protobuf message to send as the request body.
	// Max decoded size: 1MB. Default: empty (zero-length payload).
	RequestPayload string `json:"request_payload,omitempty"`

	// PayloadFormat selects payload interpretation: "raw" (base64) or "proto_json".
	// Default: "raw" (backward compatible).
	PayloadFormat string `json:"payload_format,omitempty"`

	// MonitorID is the monitor's UUID, populated by the scheduler before calling Check.
	// Used internally for proto source lookups when payload_format is "proto_json".
	MonitorID uuid.UUID `json:"monitor_id,omitempty"`
}

// GRPCChecker implements the Checker interface for gRPC monitors.
type GRPCChecker struct {
	queries *db.Queries
}

// NewGRPCChecker creates a GRPCChecker with access to the database for proto source lookups.
func NewGRPCChecker(queries *db.Queries) *GRPCChecker {
	return &GRPCChecker{queries: queries}
}

// Check executes a gRPC health check against the given target.
func (g *GRPCChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	start := time.Now()
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	// Parse and validate settings.
	s := parseGRPCSettings(settings)

	if err := validateServiceMethod(s.ServiceMethod); err != nil {
		result.State = "down"
		result.Error = err.Error()
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}
	if err := validateTLSMode(s.TLSMode); err != nil {
		result.State = "down"
		result.Error = err.Error()
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}
	if err := validateMetadata(s.Metadata); err != nil {
		result.State = "down"
		result.Error = err.Error()
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}
	if err := validateExpectedStatuses(s.ExpectedStatuses); err != nil {
		result.State = "down"
		result.Error = err.Error()
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}
	reqBytes, err := resolvePayload(ctx, g.queries, s)
	if err != nil {
		result.State = "down"
		result.Error = err.Error()
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}
	if reqBytes == nil {
		reqBytes = []byte{}
	}

	// Determine the full method to invoke.
	fullMethod := s.ServiceMethod
	if strings.TrimSpace(fullMethod) == "" {
		fullMethod = "grpc.health.v1.Health/Check"
	}

	// Build gRPC dial options.
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.ForceCodec(rawCodec{})),
	}
	switch s.TLSMode {
	case "plaintext":
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case "tls":
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	case "tls_skip_verify":
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true,
		})))
	}

	// Create the gRPC client connection (lazy dial).
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("grpc dial: %v", err)
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}
	defer conn.Close()

	// Build outgoing metadata context if metadata is provided.
	callCtx := ctx
	if len(s.Metadata) > 0 {
		md := metadata.New(s.Metadata)
		callCtx = metadata.NewOutgoingContext(ctx, md)
	}

	// Invoke the unary RPC. We use grpc.Peer to extract TLS info from the connection.
	var respBytes []byte
	var p peer.Peer
	invokeErr := conn.Invoke(callCtx, "/"+fullMethod, reqBytes, &respBytes, grpc.Peer(&p))

	// Compute latency (dial + RPC).
	result.LatencyMs = int32(time.Since(start).Milliseconds())

	// Extract TLS peer certificates if available.
	if s.TLSMode == "tls" || s.TLSMode == "tls_skip_verify" {
		if p.AuthInfo != nil {
			if tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo); ok {
				if len(tlsInfo.State.PeerCertificates) > 0 {
					leaf := tlsInfo.State.PeerCertificates[0]
					daysRemaining := int32(time.Until(leaf.NotAfter).Hours() / 24)
					result.SSLDaysRemaining = &daysRemaining

					// Check SSL expiry threshold.
					if s.SSLExpiryThreshold > 0 && int(daysRemaining) <= s.SSLExpiryThreshold {
						result.State = "down"
						result.Error = fmt.Sprintf("certificate expires in %d days (threshold: %d days)",
							daysRemaining, s.SSLExpiryThreshold)
						return result
					}
				}
			}
		}
	}

	// Extract gRPC status code from the invoke error.
	st, _ := status.FromError(invokeErr)
	code := int(st.Code())

	// Compare against expected statuses.
	expected := false
	for _, ec := range s.ExpectedStatuses {
		if code == ec {
			expected = true
			break
		}
	}

	if expected {
		result.State = "up"
	} else {
		result.State = "down"
		if invokeErr != nil {
			result.Error = fmt.Sprintf("unexpected gRPC status %d %s: %s", code, st.Code().String(), st.Message())
		} else {
			result.Error = fmt.Sprintf("unexpected gRPC status %d %s", code, st.Code().String())
		}
	}

	return result
}

// parseGRPCSettings unmarshals settings JSON and applies defaults.
func parseGRPCSettings(settings json.RawMessage) GRPCSettings {
	s := GRPCSettings{}
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &s)
	}
	if s.TLSMode == "" {
		s.TLSMode = "tls"
	}
	if len(s.ExpectedStatuses) == 0 {
		s.ExpectedStatuses = []int{0}
	}
	return s
}

// metadataKeyRegex matches valid metadata keys: lowercase alphanumeric, hyphen, underscore, dot.
var metadataKeyRegex = regexp.MustCompile(`^[a-z0-9._-]+$`)

// maxPayloadSize is the maximum decoded request payload size (1MB).
const maxPayloadSize = 1048576

// validateServiceMethod validates the service/method format.
// Whitespace-only values are treated as unset (returns nil).
// Otherwise must have exactly one "/", both segments non-empty, combined length ≤ 512.
func validateServiceMethod(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return fmt.Errorf("service_method must contain exactly one '/' separator")
	}

	service, method := parts[0], parts[1]
	if service == "" {
		return fmt.Errorf("service_method service segment (before '/') must not be empty")
	}
	if method == "" {
		return fmt.Errorf("service_method method segment (after '/') must not be empty")
	}
	if len(s) > 512 {
		return fmt.Errorf("service_method combined length must not exceed 512 characters, got %d", len(s))
	}

	return nil
}

// validateTLSMode validates the TLS mode value.
func validateTLSMode(mode string) error {
	switch mode {
	case "plaintext", "tls", "tls_skip_verify":
		return nil
	default:
		return fmt.Errorf("tls_mode must be one of: plaintext, tls, tls_skip_verify; got %q", mode)
	}
}

// validateMetadata validates metadata keys and values.
// Max 20 entries, key max 128 chars, key must match [a-z0-9._-]+, no "grpc-" prefix,
// keys ending in "-bin" require valid base64 values, value max 4096 chars.
func validateMetadata(md map[string]string) error {
	if len(md) > 20 {
		return fmt.Errorf("metadata must have at most 20 entries, got %d", len(md))
	}

	for key, value := range md {
		if len(key) > 128 {
			return fmt.Errorf("metadata key %q exceeds maximum length of 128 characters", key)
		}

		if !metadataKeyRegex.MatchString(key) {
			return fmt.Errorf("metadata key %q contains invalid characters; must be lowercase alphanumeric, hyphen, underscore, or dot", key)
		}

		if strings.HasPrefix(key, "grpc-") {
			return fmt.Errorf("metadata key %q must not start with reserved prefix \"grpc-\"", key)
		}

		if len(value) > 4096 {
			return fmt.Errorf("metadata value for key %q exceeds maximum length of 4096 characters", key)
		}

		if strings.HasSuffix(key, "-bin") {
			if _, err := base64.StdEncoding.Strict().DecodeString(value); err != nil {
				return fmt.Errorf("metadata key %q ends with \"-bin\" but value is not valid base64: %v", key, err)
			}
		}
	}

	return nil
}

// validateExpectedStatuses validates expected gRPC status codes.
// Each value must be in [0, 16], max 17 entries.
func validateExpectedStatuses(statuses []int) error {
	if len(statuses) > 17 {
		return fmt.Errorf("expected_statuses must have at most 17 entries, got %d", len(statuses))
	}

	for _, code := range statuses {
		if code < 0 || code > 16 {
			return fmt.Errorf("expected_statuses value %d is out of range; must be 0–16", code)
		}
	}

	return nil
}

// validateRequestPayload validates and decodes a base64-encoded request payload.
// Returns the decoded bytes on success. Decoded size must be ≤ 1MB.
func validateRequestPayload(payload string) ([]byte, error) {
	if payload == "" {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("request_payload is not valid base64: %v", err)
	}

	if len(decoded) > maxPayloadSize {
		return nil, fmt.Errorf("request_payload decoded size %d exceeds maximum of %d bytes (1MB)", len(decoded), maxPayloadSize)
	}

	return decoded, nil
}

// rawCodec is a gRPC codec that passes bytes through without protobuf marshaling.
// This allows invoking any gRPC method without generated stubs.
type rawCodec struct{}

// Ensure rawCodec implements the encoding.Codec interface.
var _ encoding.Codec = rawCodec{}

func (rawCodec) Marshal(v interface{}) ([]byte, error) {
	return v.([]byte), nil
}

func (rawCodec) Unmarshal(data []byte, v interface{}) error {
	*v.(*[]byte) = data
	return nil
}

func (rawCodec) Name() string {
	return "raw"
}

// resolvePayload returns binary protobuf bytes from either base64 or Proto JSON format.
// When format is "proto_json", it loads the proto source from DB, resolves the
// message descriptor, and converts JSON to binary.
func resolvePayload(ctx context.Context, queries *db.Queries, settings GRPCSettings) ([]byte, error) {
	// Default or "raw" → existing base64 decode behavior.
	if settings.PayloadFormat == "" || settings.PayloadFormat == "raw" {
		return validateRequestPayload(settings.RequestPayload)
	}

	if settings.PayloadFormat != "proto_json" {
		return nil, fmt.Errorf("unsupported payload_format: %q", settings.PayloadFormat)
	}

	// proto_json mode requires a database queries instance.
	if queries == nil {
		return nil, fmt.Errorf("proto source required for proto_json payload format")
	}

	// proto_json mode requires a valid monitor ID for proto source lookup.
	if settings.MonitorID == uuid.Nil {
		return nil, fmt.Errorf("proto source required for proto_json payload format")
	}

	// Load proto source from DB.
	protoSource, err := queries.GetProtoSource(ctx, settings.MonitorID)
	if err != nil {
		return nil, fmt.Errorf("proto source required for proto_json payload format")
	}

	// Parse the stored descriptor bytes into FileDescriptorSet.
	registry := proto.NewRegistry()
	fds, err := registry.ParseFileDescriptorSet(protoSource.DescriptorBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stored proto source: %w", err)
	}

	// Resolve the service method's input message type.
	serviceMethod := settings.ServiceMethod
	if strings.TrimSpace(serviceMethod) == "" {
		return nil, fmt.Errorf("service_method is required for proto_json payload format")
	}

	inputType, err := resolveInputType(fds, serviceMethod)
	if err != nil {
		return nil, err
	}

	// Resolve the message descriptor for the input type.
	msgDesc, err := proto.ResolveMessageDescriptor(fds, inputType)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve message type %q: %w", inputType, err)
	}

	// Validate payload size ≤ 1MB before conversion.
	if len(settings.RequestPayload) > maxPayloadSize {
		return nil, fmt.Errorf("request_payload size %d exceeds maximum of %d bytes (1MB)", len(settings.RequestPayload), maxPayloadSize)
	}

	// Convert Proto JSON to binary protobuf.
	data, err := registry.ProtoJSONToBytes(msgDesc, []byte(settings.RequestPayload))
	if err != nil {
		return nil, fmt.Errorf("proto JSON conversion failed: %w", err)
	}

	// Validate converted payload size ≤ 1MB.
	if len(data) > maxPayloadSize {
		return nil, fmt.Errorf("request_payload decoded size %d exceeds maximum of %d bytes (1MB)", len(data), maxPayloadSize)
	}

	return data, nil
}

// resolveInputType extracts the input message type from the FileDescriptorSet
// for the given service/method string (format: "package.Service/Method").
func resolveInputType(fds *descriptorpb.FileDescriptorSet, serviceMethod string) (string, error) {
	parts := strings.SplitN(serviceMethod, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid service_method format %q: expected \"Service/Method\"", serviceMethod)
	}
	serviceName, methodName := parts[0], parts[1]

	// Extract metadata to find the input type for the method.
	meta, err := proto.ExtractMetadata(fds)
	if err != nil {
		return "", fmt.Errorf("failed to extract proto metadata: %w", err)
	}

	for _, svc := range meta.Services {
		if svc.FullName == serviceName {
			for _, m := range svc.Methods {
				if m.Name == methodName {
					return m.InputType, nil
				}
			}
		}
	}

	return "", fmt.Errorf("method %q not found in service %q", methodName, serviceName)
}
