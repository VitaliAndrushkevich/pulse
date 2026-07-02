package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	protolib "github.com/VitaliAndrushkevich/pulse/internal/proto"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// --- Fake DB for resolvePayload integration tests ---

// resolvePayloadFakeDB implements db.DBTX for resolvePayload tests.
type resolvePayloadFakeDB struct {
	protoSources map[uuid.UUID]db.ProtoSource
}

func newResolvePayloadFakeDB() *resolvePayloadFakeDB {
	return &resolvePayloadFakeDB{
		protoSources: make(map[uuid.UUID]db.ProtoSource),
	}
}

func (f *resolvePayloadFakeDB) addProtoSource(monitorID uuid.UUID, ps db.ProtoSource) {
	f.protoSources[monitorID] = ps
}

func (f *resolvePayloadFakeDB) Exec(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), nil
}

func (f *resolvePayloadFakeDB) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, pgx.ErrNoRows
}

func (f *resolvePayloadFakeDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	// Handle GetProtoSource query.
	if strings.Contains(sql, "proto_sources") {
		if len(args) > 0 {
			monitorID, ok := args[0].(uuid.UUID)
			if ok {
				if ps, found := f.protoSources[monitorID]; found {
					return &fakeProtoSourceRow{ps: ps}
				}
			}
		}
		return &fakeProtoSourceRow{err: pgx.ErrNoRows}
	}
	return &fakeProtoSourceRow{err: pgx.ErrNoRows}
}

// fakeProtoSourceRow implements pgx.Row for returning proto source data.
type fakeProtoSourceRow struct {
	ps  db.ProtoSource
	err error
}

func (r *fakeProtoSourceRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) >= 7 {
		*dest[0].(*uuid.UUID) = r.ps.ID
		*dest[1].(*uuid.UUID) = r.ps.MonitorID
		*dest[2].(*string) = r.ps.SourceType
		*dest[3].(*[]byte) = r.ps.DescriptorBytes
		*dest[4].(*json.RawMessage) = r.ps.Metadata
		*dest[5].(*time.Time) = r.ps.CreatedAt
		*dest[6].(*time.Time) = r.ps.UpdatedAt
	}
	return nil
}

// --- Test helpers ---

// makeTestFileDescriptorSet creates a FileDescriptorSet with a service and message
// containing a string field "name" and an int32 field "id".
func makeTestFileDescriptorSet() (*descriptorpb.FileDescriptorSet, []byte) {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    protoStrPtr("test.proto"),
				Package: protoStrPtr("testpkg"),
				Syntax:  protoStrPtr("proto3"),
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: protoStrPtr("TestService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       protoStrPtr("TestMethod"),
								InputType:  protoStrPtr(".testpkg.TestRequest"),
								OutputType: protoStrPtr(".testpkg.TestResponse"),
							},
						},
					},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: protoStrPtr("TestRequest"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     protoStrPtr("name"),
								Number:   protoInt32Ptr(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								JsonName: protoStrPtr("name"),
							},
							{
								Name:     protoStrPtr("id"),
								Number:   protoInt32Ptr(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								JsonName: protoStrPtr("id"),
							},
						},
					},
					{
						Name: protoStrPtr("TestResponse"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     protoStrPtr("message"),
								Number:   protoInt32Ptr(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								JsonName: protoStrPtr("message"),
							},
						},
					},
				},
			},
		},
	}
	data, _ := proto.Marshal(fds)
	return fds, data
}

func protoStrPtr(s string) *string { return &s }
func protoInt32Ptr(i int32) *int32 { return &i }

// --- Integration Tests for resolvePayload ---

// TestResolvePayload_ProtoJSON_ValidSchemaAndPayload tests that proto_json with a valid
// schema and conforming JSON payload correctly converts to binary protobuf bytes,
// and those bytes can be deserialized back.
// Validates: Requirements 3.1, 3.3
func TestResolvePayload_ProtoJSON_ValidSchemaAndPayload(t *testing.T) {
	fdb := newResolvePayloadFakeDB()
	monitorID := uuid.New()

	fds, fdsBytes := makeTestFileDescriptorSet()

	fdb.addProtoSource(monitorID, db.ProtoSource{
		ID:              uuid.New(),
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: fdsBytes,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})

	queries := db.New(fdb)

	settings := GRPCSettings{
		PayloadFormat:  "proto_json",
		RequestPayload: `{"name": "hello", "id": 42}`,
		ServiceMethod:  "testpkg.TestService/TestMethod",
		MonitorID:      monitorID,
	}

	ctx := context.Background()
	result, err := resolvePayload(ctx, queries, settings)
	if err != nil {
		t.Fatalf("resolvePayload returned unexpected error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("resolvePayload returned empty bytes")
	}

	// Verify the binary bytes can be deserialized back correctly.
	registry := protolib.NewRegistry()
	msgDesc, err := protolib.ResolveMessageDescriptor(fds, "testpkg.TestRequest")
	if err != nil {
		t.Fatalf("failed to resolve message descriptor: %v", err)
	}

	msg := dynamicpb.NewMessage(msgDesc)
	if err := proto.Unmarshal(result, msg); err != nil {
		t.Fatalf("failed to unmarshal binary result: %v", err)
	}

	// Convert back to JSON and verify field values.
	jsonBytes, err := registry.BytesToProtoJSON(msgDesc, result)
	if err != nil {
		t.Fatalf("failed to convert binary back to Proto JSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}

	if parsed["name"] != "hello" {
		t.Errorf("expected name='hello', got %v", parsed["name"])
	}
	// Proto JSON serializes int32 as a JSON number.
	if id, ok := parsed["id"].(float64); !ok || id != 42 {
		t.Errorf("expected id=42, got %v", parsed["id"])
	}
}

// TestResolvePayload_ProtoJSON_EmptyPayloadFields tests that proto_json with valid schema
// and empty/zero-value fields works correctly.
// Validates: Requirements 3.1
func TestResolvePayload_ProtoJSON_EmptyPayloadFields(t *testing.T) {
	fdb := newResolvePayloadFakeDB()
	monitorID := uuid.New()

	_, fdsBytes := makeTestFileDescriptorSet()

	fdb.addProtoSource(monitorID, db.ProtoSource{
		ID:              uuid.New(),
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: fdsBytes,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})

	queries := db.New(fdb)

	// Empty JSON object — all fields at zero values.
	settings := GRPCSettings{
		PayloadFormat:  "proto_json",
		RequestPayload: `{}`,
		ServiceMethod:  "testpkg.TestService/TestMethod",
		MonitorID:      monitorID,
	}

	ctx := context.Background()
	result, err := resolvePayload(ctx, queries, settings)
	if err != nil {
		t.Fatalf("resolvePayload returned unexpected error: %v", err)
	}

	// Empty proto message serializes to zero bytes in proto3.
	// This is valid — an empty message with all default values.
	if result == nil {
		t.Fatal("resolvePayload returned nil bytes")
	}
}

// TestResolvePayload_ProtoJSON_InvalidJSON tests that proto_json with JSON containing
// unknown fields returns an error.
// Validates: Requirements 3.2
func TestResolvePayload_ProtoJSON_InvalidJSON(t *testing.T) {
	fdb := newResolvePayloadFakeDB()
	monitorID := uuid.New()

	_, fdsBytes := makeTestFileDescriptorSet()

	fdb.addProtoSource(monitorID, db.ProtoSource{
		ID:              uuid.New(),
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: fdsBytes,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})

	queries := db.New(fdb)

	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "unknown field",
			payload: `{"unknown_field": "value"}`,
		},
		{
			name:    "type mismatch string for int",
			payload: `{"id": "not_a_number"}`,
		},
		{
			name:    "malformed JSON",
			payload: `{invalid json`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := GRPCSettings{
				PayloadFormat:  "proto_json",
				RequestPayload: tc.payload,
				ServiceMethod:  "testpkg.TestService/TestMethod",
				MonitorID:      monitorID,
			}

			ctx := context.Background()
			_, err := resolvePayload(ctx, queries, settings)
			if err == nil {
				t.Fatal("expected error for invalid JSON payload, got nil")
			}
			if !strings.Contains(err.Error(), "proto JSON conversion failed") {
				t.Errorf("expected error to mention 'proto JSON conversion failed', got: %v", err)
			}
		})
	}
}

// TestResolvePayload_ProtoJSON_MissingProtoSource tests that proto_json without
// a stored proto source returns "proto source required" error.
// Validates: Requirements 3.6
func TestResolvePayload_ProtoJSON_MissingProtoSource(t *testing.T) {
	fdb := newResolvePayloadFakeDB()
	monitorID := uuid.New()
	// Do NOT add any proto source for this monitor.

	queries := db.New(fdb)

	settings := GRPCSettings{
		PayloadFormat:  "proto_json",
		RequestPayload: `{"name": "hello"}`,
		ServiceMethod:  "testpkg.TestService/TestMethod",
		MonitorID:      monitorID,
	}

	ctx := context.Background()
	_, err := resolvePayload(ctx, queries, settings)
	if err == nil {
		t.Fatal("expected error for missing proto source, got nil")
	}
	if !strings.Contains(err.Error(), "proto source required") {
		t.Errorf("expected error to contain 'proto source required', got: %v", err)
	}
}

// TestResolvePayload_ProtoJSON_NilQueries tests that proto_json without a queries
// instance returns "proto source required" error.
// Validates: Requirements 3.6
func TestResolvePayload_ProtoJSON_NilQueries(t *testing.T) {
	monitorID := uuid.New()

	settings := GRPCSettings{
		PayloadFormat:  "proto_json",
		RequestPayload: `{"name": "hello"}`,
		ServiceMethod:  "testpkg.TestService/TestMethod",
		MonitorID:      monitorID,
	}

	ctx := context.Background()
	_, err := resolvePayload(ctx, nil, settings)
	if err == nil {
		t.Fatal("expected error for nil queries, got nil")
	}
	if !strings.Contains(err.Error(), "proto source required") {
		t.Errorf("expected error to contain 'proto source required', got: %v", err)
	}
}

// TestResolvePayload_ProtoJSON_NilMonitorID tests that proto_json with a zero monitor ID
// returns "proto source required" error.
// Validates: Requirements 3.6
func TestResolvePayload_ProtoJSON_NilMonitorID(t *testing.T) {
	fdb := newResolvePayloadFakeDB()
	queries := db.New(fdb)

	settings := GRPCSettings{
		PayloadFormat:  "proto_json",
		RequestPayload: `{"name": "hello"}`,
		ServiceMethod:  "testpkg.TestService/TestMethod",
		MonitorID:      uuid.Nil, // zero UUID
	}

	ctx := context.Background()
	_, err := resolvePayload(ctx, queries, settings)
	if err == nil {
		t.Fatal("expected error for nil monitor ID, got nil")
	}
	if !strings.Contains(err.Error(), "proto source required") {
		t.Errorf("expected error to contain 'proto source required', got: %v", err)
	}
}

// TestResolvePayload_Raw_ValidBase64 tests that "raw" format with valid base64
// correctly decodes to the original bytes.
// Validates: Requirements 3.5
func TestResolvePayload_Raw_ValidBase64(t *testing.T) {
	originalBytes := []byte{0x0a, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f} // some binary data
	encoded := base64.StdEncoding.EncodeToString(originalBytes)

	settings := GRPCSettings{
		PayloadFormat:  "raw",
		RequestPayload: encoded,
	}

	ctx := context.Background()
	result, err := resolvePayload(ctx, nil, settings)
	if err != nil {
		t.Fatalf("resolvePayload returned unexpected error: %v", err)
	}

	if len(result) != len(originalBytes) {
		t.Fatalf("length mismatch: expected %d, got %d", len(originalBytes), len(result))
	}
	for i := range originalBytes {
		if result[i] != originalBytes[i] {
			t.Fatalf("byte mismatch at index %d: expected 0x%02x, got 0x%02x", i, originalBytes[i], result[i])
		}
	}
}

// TestResolvePayload_Raw_EmptyPayload tests that "raw" format with empty payload returns nil.
// Validates: Requirements 3.5
func TestResolvePayload_Raw_EmptyPayload(t *testing.T) {
	settings := GRPCSettings{
		PayloadFormat:  "raw",
		RequestPayload: "",
	}

	ctx := context.Background()
	result, err := resolvePayload(ctx, nil, settings)
	if err != nil {
		t.Fatalf("resolvePayload returned unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty payload, got %v", result)
	}
}

// TestResolvePayload_EmptyFormat_BehavesAsRaw tests that an empty payload_format
// behaves the same as "raw" (backward compatible).
// Validates: Requirements 3.5
func TestResolvePayload_EmptyFormat_BehavesAsRaw(t *testing.T) {
	originalBytes := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	encoded := base64.StdEncoding.EncodeToString(originalBytes)

	settings := GRPCSettings{
		PayloadFormat:  "", // empty format → treated as "raw"
		RequestPayload: encoded,
	}

	ctx := context.Background()
	result, err := resolvePayload(ctx, nil, settings)
	if err != nil {
		t.Fatalf("resolvePayload returned unexpected error: %v", err)
	}

	if len(result) != len(originalBytes) {
		t.Fatalf("length mismatch: expected %d, got %d", len(originalBytes), len(result))
	}
	for i := range originalBytes {
		if result[i] != originalBytes[i] {
			t.Fatalf("byte mismatch at index %d: expected 0x%02x, got 0x%02x", i, originalBytes[i], result[i])
		}
	}
}

// TestResolvePayload_Raw_PayloadSizeExceeded tests that "raw" format with decoded payload
// exceeding 1MB returns an error.
// Validates: Requirements 3.7
func TestResolvePayload_Raw_PayloadSizeExceeded(t *testing.T) {
	// Create a payload slightly over 1MB.
	largeData := make([]byte, maxPayloadSize+1)
	for i := range largeData {
		largeData[i] = 0x42
	}
	encoded := base64.StdEncoding.EncodeToString(largeData)

	settings := GRPCSettings{
		PayloadFormat:  "raw",
		RequestPayload: encoded,
	}

	ctx := context.Background()
	_, err := resolvePayload(ctx, nil, settings)
	if err == nil {
		t.Fatal("expected error for payload exceeding 1MB, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected error to mention 'exceeds maximum', got: %v", err)
	}
}

// TestResolvePayload_ProtoJSON_PayloadSizeExceeded tests that proto_json with a JSON
// payload exceeding 1MB returns an error.
// Validates: Requirements 3.7
func TestResolvePayload_ProtoJSON_PayloadSizeExceeded(t *testing.T) {
	fdb := newResolvePayloadFakeDB()
	monitorID := uuid.New()

	_, fdsBytes := makeTestFileDescriptorSet()

	fdb.addProtoSource(monitorID, db.ProtoSource{
		ID:              uuid.New(),
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: fdsBytes,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})

	queries := db.New(fdb)

	// Create a JSON payload over 1MB by building a huge string value.
	largeValue := strings.Repeat("x", maxPayloadSize+1)
	payload := `{"name": "` + largeValue + `"}`

	settings := GRPCSettings{
		PayloadFormat:  "proto_json",
		RequestPayload: payload,
		ServiceMethod:  "testpkg.TestService/TestMethod",
		MonitorID:      monitorID,
	}

	ctx := context.Background()
	_, err := resolvePayload(ctx, queries, settings)
	if err == nil {
		t.Fatal("expected error for proto_json payload exceeding 1MB, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected error to mention 'exceeds maximum', got: %v", err)
	}
}

// TestResolvePayload_ProtoJSON_RoundTrip tests the full round-trip: JSON → binary → JSON
// verifying data integrity through the conversion pipeline.
// Validates: Requirements 3.1
func TestResolvePayload_ProtoJSON_RoundTrip(t *testing.T) {
	fdb := newResolvePayloadFakeDB()
	monitorID := uuid.New()

	fds, fdsBytes := makeTestFileDescriptorSet()

	fdb.addProtoSource(monitorID, db.ProtoSource{
		ID:              uuid.New(),
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: fdsBytes,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})

	queries := db.New(fdb)

	// Input JSON.
	inputJSON := `{"name": "world", "id": 99}`

	settings := GRPCSettings{
		PayloadFormat:  "proto_json",
		RequestPayload: inputJSON,
		ServiceMethod:  "testpkg.TestService/TestMethod",
		MonitorID:      monitorID,
	}

	ctx := context.Background()
	binaryResult, err := resolvePayload(ctx, queries, settings)
	if err != nil {
		t.Fatalf("resolvePayload returned unexpected error: %v", err)
	}

	// Deserialize binary back to a dynamic message.
	msgDesc, err := protolib.ResolveMessageDescriptor(fds, "testpkg.TestRequest")
	if err != nil {
		t.Fatalf("failed to resolve message descriptor: %v", err)
	}

	msg := dynamicpb.NewMessage(msgDesc)
	if err := proto.Unmarshal(binaryResult, msg); err != nil {
		t.Fatalf("failed to unmarshal binary: %v", err)
	}

	// Convert back to JSON.
	marshalOpts := protojson.MarshalOptions{EmitUnpopulated: false}
	roundTripJSON, err := marshalOpts.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal back to JSON: %v", err)
	}

	// Parse both JSONs to compare semantically.
	var original, roundTripped map[string]interface{}
	if err := json.Unmarshal([]byte(inputJSON), &original); err != nil {
		t.Fatalf("failed to parse input JSON: %v", err)
	}
	if err := json.Unmarshal(roundTripJSON, &roundTripped); err != nil {
		t.Fatalf("failed to parse round-trip JSON: %v", err)
	}

	// Verify "name" field.
	if roundTripped["name"] != original["name"] {
		t.Errorf("name mismatch: expected %v, got %v", original["name"], roundTripped["name"])
	}

	// Verify "id" field (JSON numbers are float64).
	originalID := original["id"].(float64)
	roundTrippedID := roundTripped["id"].(float64)
	if originalID != roundTrippedID {
		t.Errorf("id mismatch: expected %v, got %v", originalID, roundTrippedID)
	}
}

// TestResolvePayload_UnsupportedFormat tests that an unsupported payload_format returns an error.
// Validates: Requirements 3.3
func TestResolvePayload_UnsupportedFormat(t *testing.T) {
	settings := GRPCSettings{
		PayloadFormat:  "xml",
		RequestPayload: "<data/>",
	}

	ctx := context.Background()
	_, err := resolvePayload(ctx, nil, settings)
	if err == nil {
		t.Fatal("expected error for unsupported payload format, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported payload_format") {
		t.Errorf("expected error to mention 'unsupported payload_format', got: %v", err)
	}
}

// Ensure unused imports are used.
var _ = protojson.MarshalOptions{}

// TestResolvePayload_ProtoJSON_MissingServiceMethod tests that proto_json
// without a service_method returns an error.
// Validates: Requirements 3.1
func TestResolvePayload_ProtoJSON_MissingServiceMethod(t *testing.T) {
	fdb := newResolvePayloadFakeDB()
	monitorID := uuid.New()

	_, fdsBytes := makeTestFileDescriptorSet()

	fdb.addProtoSource(monitorID, db.ProtoSource{
		ID:              uuid.New(),
		MonitorID:       monitorID,
		SourceType:      "upload",
		DescriptorBytes: fdsBytes,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})

	queries := db.New(fdb)

	settings := GRPCSettings{
		PayloadFormat:  "proto_json",
		ServiceMethod:  "", // empty — should fail
		RequestPayload: `{"name": "hello"}`,
		MonitorID:      monitorID,
	}

	ctx := context.Background()
	_, err := resolvePayload(ctx, queries, settings)
	if err == nil {
		t.Fatal("expected error for missing service_method, got nil")
	}
	if !strings.Contains(err.Error(), "service_method is required") {
		t.Errorf("expected 'service_method is required' error, got: %v", err)
	}
}

// --- Tests for resolveInputType ---

// TestResolveInputType_ValidServiceMethod tests that resolveInputType correctly
// returns the input message type for a valid service/method.
func TestResolveInputType_ValidServiceMethod(t *testing.T) {
	fds, _ := makeTestFileDescriptorSet()

	inputType, err := resolveInputType(fds, "testpkg.TestService/TestMethod")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if inputType != "testpkg.TestRequest" {
		t.Fatalf("expected input type 'testpkg.TestRequest', got: %q", inputType)
	}
}

// TestResolveInputType_NonExistentMethod tests that resolveInputType returns
// an error when the method does not exist.
func TestResolveInputType_NonExistentMethod(t *testing.T) {
	fds, _ := makeTestFileDescriptorSet()

	_, err := resolveInputType(fds, "testpkg.TestService/NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent method, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// TestResolveInputType_NonExistentService tests that resolveInputType returns
// an error when the service does not exist.
func TestResolveInputType_NonExistentService(t *testing.T) {
	fds, _ := makeTestFileDescriptorSet()

	_, err := resolveInputType(fds, "testpkg.Unknown/TestMethod")
	if err == nil {
		t.Fatal("expected error for non-existent service, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// TestResolveInputType_InvalidFormat tests that resolveInputType returns
// an error for malformed service_method strings.
func TestResolveInputType_InvalidFormat(t *testing.T) {
	fds, _ := makeTestFileDescriptorSet()

	cases := []struct {
		name          string
		serviceMethod string
	}{
		{"no slash", "testpkg.TestService.TestMethod"},
		{"empty", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveInputType(fds, tc.serviceMethod)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.serviceMethod)
			}
		})
	}
}
