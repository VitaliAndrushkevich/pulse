package proto

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestParseFileDescriptorSet_Valid(t *testing.T) {
	// Create a minimal valid FileDescriptorSet.
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:   strPtr("test.proto"),
				Syntax: strPtr("proto3"),
			},
		},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("failed to marshal test FDS: %v", err)
	}

	reg := NewRegistry()
	result, err := reg.ParseFileDescriptorSet(data)
	if err != nil {
		t.Fatalf("ParseFileDescriptorSet returned error: %v", err)
	}
	if len(result.GetFile()) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.GetFile()))
	}
	if result.GetFile()[0].GetName() != "test.proto" {
		t.Errorf("expected file name 'test.proto', got %q", result.GetFile()[0].GetName())
	}
}

func TestParseFileDescriptorSet_Empty(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.ParseFileDescriptorSet(nil)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseFileDescriptorSet_InvalidBinary(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.ParseFileDescriptorSet([]byte("not a valid protobuf"))
	if err == nil {
		t.Fatal("expected error for invalid binary")
	}
}

func TestParseFileDescriptorSet_NoFiles(t *testing.T) {
	// Valid protobuf but empty FileDescriptorSet.
	fds := &descriptorpb.FileDescriptorSet{}
	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	reg := NewRegistry()
	_, err = reg.ParseFileDescriptorSet(data)
	if err == nil {
		t.Fatal("expected error for empty FileDescriptorSet")
	}
}

func TestParseProtoFiles_Valid(t *testing.T) {
	files := map[string][]byte{
		"test.proto": []byte(`
syntax = "proto3";
package testpkg;

message HelloRequest {
  string name = 1;
}

message HelloResponse {
  string message = 1;
}

service Greeter {
  rpc SayHello (HelloRequest) returns (HelloResponse);
}
`),
	}

	reg := NewRegistry()
	result, err := reg.ParseProtoFiles(files)
	if err != nil {
		t.Fatalf("ParseProtoFiles returned error: %v", err)
	}
	if len(result.GetFile()) == 0 {
		t.Fatal("expected at least one file in result")
	}

	// Find our test file in the result set.
	var found bool
	for _, f := range result.GetFile() {
		if f.GetName() == "test.proto" {
			found = true
			if len(f.GetService()) != 1 {
				t.Errorf("expected 1 service, got %d", len(f.GetService()))
			}
			if len(f.GetMessageType()) != 2 {
				t.Errorf("expected 2 messages, got %d", len(f.GetMessageType()))
			}
		}
	}
	if !found {
		t.Error("test.proto not found in result")
	}
}

func TestParseProtoFiles_UnresolvedImport(t *testing.T) {
	files := map[string][]byte{
		"service.proto": []byte(`
syntax = "proto3";
package testpkg;
import "missing.proto";

message Request {
  string name = 1;
}
`),
	}

	reg := NewRegistry()
	_, err := reg.ParseProtoFiles(files)
	if err == nil {
		t.Fatal("expected error for unresolved import")
	}
	if !containsSubstring(err.Error(), "missing.proto") {
		t.Errorf("error should mention unresolved import path, got: %v", err)
	}
}

func TestParseProtoFiles_InvalidSyntax(t *testing.T) {
	files := map[string][]byte{
		"bad.proto": []byte(`this is not valid proto syntax`),
	}

	reg := NewRegistry()
	_, err := reg.ParseProtoFiles(files)
	if err == nil {
		t.Fatal("expected error for invalid syntax")
	}
}

func TestParseProtoFiles_Empty(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.ParseProtoFiles(nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

func TestParseProtoFiles_WellKnownImports(t *testing.T) {
	files := map[string][]byte{
		"test.proto": []byte(`
syntax = "proto3";
package testpkg;
import "google/protobuf/timestamp.proto";

message Event {
  string name = 1;
  google.protobuf.Timestamp created_at = 2;
}
`),
	}

	reg := NewRegistry()
	result, err := reg.ParseProtoFiles(files)
	if err != nil {
		t.Fatalf("ParseProtoFiles should resolve well-known imports, got error: %v", err)
	}
	if len(result.GetFile()) == 0 {
		t.Fatal("expected files in result")
	}
}

func TestParseProtoFiles_MultipleFiles(t *testing.T) {
	files := map[string][]byte{
		"common.proto": []byte(`
syntax = "proto3";
package common;

message Pagination {
  int32 page = 1;
  int32 limit = 2;
}
`),
		"service.proto": []byte(`
syntax = "proto3";
package myservice;
import "common.proto";

message ListRequest {
  common.Pagination pagination = 1;
}

message ListResponse {
  repeated string items = 1;
}

service ItemService {
  rpc List (ListRequest) returns (ListResponse);
}
`),
	}

	reg := NewRegistry()
	result, err := reg.ParseProtoFiles(files)
	if err != nil {
		t.Fatalf("ParseProtoFiles returned error: %v", err)
	}

	// Should contain both files (and potentially dependencies).
	fileNames := make(map[string]bool)
	for _, f := range result.GetFile() {
		fileNames[f.GetName()] = true
	}
	if !fileNames["common.proto"] {
		t.Error("missing common.proto in result")
	}
	if !fileNames["service.proto"] {
		t.Error("missing service.proto in result")
	}
}

func strPtr(s string) *string {
	return &s
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// buildTestFDS is a convenience helper that compiles a proto source string into a FileDescriptorSet.
func buildTestFDS(t *testing.T, protoSource string) *descriptorpb.FileDescriptorSet {
	t.Helper()
	reg := NewRegistry()
	fds, err := reg.ParseProtoFiles(map[string][]byte{"test.proto": []byte(protoSource)})
	if err != nil {
		t.Fatalf("failed to build test FDS: %v", err)
	}
	return fds
}

// --- ExtractMetadata Tests ---

func TestExtractMetadata_MultipleServicesMultipleMethods(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package multi;

message ReqA { string name = 1; }
message RespA { string result = 1; }
message ReqB { int32 id = 1; }
message RespB { bool ok = 1; }
message ReqC { bytes data = 1; }
message RespC { string hash = 1; }

service ServiceAlpha {
  rpc MethodOne (ReqA) returns (RespA);
  rpc MethodTwo (ReqB) returns (RespB);
}

service ServiceBeta {
  rpc MethodThree (ReqC) returns (RespC);
}
`)

	meta, err := ExtractMetadata(fds)
	if err != nil {
		t.Fatalf("ExtractMetadata returned error: %v", err)
	}

	// Verify all services are extracted.
	serviceNames := make(map[string]bool)
	for _, svc := range meta.Services {
		serviceNames[svc.FullName] = true
	}
	if !serviceNames["multi.ServiceAlpha"] {
		t.Error("missing service multi.ServiceAlpha")
	}
	if !serviceNames["multi.ServiceBeta"] {
		t.Error("missing service multi.ServiceBeta")
	}

	// Verify methods for ServiceAlpha.
	var alphaService *ProtoService
	for i := range meta.Services {
		if meta.Services[i].FullName == "multi.ServiceAlpha" {
			alphaService = &meta.Services[i]
			break
		}
	}
	if alphaService == nil {
		t.Fatal("ServiceAlpha not found in metadata")
	}
	if len(alphaService.Methods) != 2 {
		t.Fatalf("expected 2 methods in ServiceAlpha, got %d", len(alphaService.Methods))
	}

	methodNames := make(map[string]bool)
	for _, m := range alphaService.Methods {
		methodNames[m.Name] = true
	}
	if !methodNames["MethodOne"] {
		t.Error("missing method MethodOne in ServiceAlpha")
	}
	if !methodNames["MethodTwo"] {
		t.Error("missing method MethodTwo in ServiceAlpha")
	}

	// Verify input/output types.
	for _, m := range alphaService.Methods {
		if m.Name == "MethodOne" {
			if m.InputType != "multi.ReqA" {
				t.Errorf("MethodOne input: want multi.ReqA, got %s", m.InputType)
			}
			if m.OutputType != "multi.RespA" {
				t.Errorf("MethodOne output: want multi.RespA, got %s", m.OutputType)
			}
		}
	}

	// Verify ServiceBeta has one method.
	var betaService *ProtoService
	for i := range meta.Services {
		if meta.Services[i].FullName == "multi.ServiceBeta" {
			betaService = &meta.Services[i]
			break
		}
	}
	if betaService == nil {
		t.Fatal("ServiceBeta not found")
	}
	if len(betaService.Methods) != 1 {
		t.Fatalf("expected 1 method in ServiceBeta, got %d", len(betaService.Methods))
	}
	if betaService.Methods[0].InputType != "multi.ReqC" {
		t.Errorf("MethodThree input: want multi.ReqC, got %s", betaService.Methods[0].InputType)
	}

	// Verify all message types are extracted.
	msgTypes := make(map[string]bool)
	for _, mt := range meta.MessageTypes {
		msgTypes[mt] = true
	}
	expectedMsgs := []string{"multi.ReqA", "multi.RespA", "multi.ReqB", "multi.RespB", "multi.ReqC", "multi.RespC"}
	for _, em := range expectedMsgs {
		if !msgTypes[em] {
			t.Errorf("missing message type %s in metadata", em)
		}
	}
}

func TestExtractMetadata_EmptyNoServices(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package empty;

message SomeMessage {
  string value = 1;
}
`)

	meta, err := ExtractMetadata(fds)
	if err != nil {
		t.Fatalf("ExtractMetadata returned error: %v", err)
	}

	if len(meta.Services) != 0 {
		t.Errorf("expected 0 services for message-only proto, got %d", len(meta.Services))
	}

	// Message type should still be extracted.
	if len(meta.MessageTypes) == 0 {
		t.Error("expected at least one message type")
	}
}

func TestExtractMetadata_NestedMessageTypes(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package nested;

message Outer {
  message Inner {
    string value = 1;
  }
  Inner inner = 1;
  string name = 2;
}

service NestedService {
  rpc DoSomething (Outer) returns (Outer);
}
`)

	meta, err := ExtractMetadata(fds)
	if err != nil {
		t.Fatalf("ExtractMetadata returned error: %v", err)
	}

	// The top-level message should be present.
	found := false
	for _, mt := range meta.MessageTypes {
		if mt == "nested.Outer" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected nested.Outer in message types")
	}

	// Service and method should be correctly extracted.
	if len(meta.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(meta.Services))
	}
	if meta.Services[0].Methods[0].InputType != "nested.Outer" {
		t.Errorf("expected input type nested.Outer, got %s", meta.Services[0].Methods[0].InputType)
	}
}

func TestExtractMetadata_NilFDS(t *testing.T) {
	_, err := ExtractMetadata(nil)
	if err == nil {
		t.Fatal("expected error for nil FileDescriptorSet")
	}
}

// --- ProtoJSONToBytes and BytesToProtoJSON Tests ---

func TestProtoJSONToBytes_RoundTrip_StringField(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package roundtrip;

message SimpleMsg {
  string name = 1;
  int32 count = 2;
  int64 big_count = 3;
  float ratio = 4;
  double precise = 5;
  bool active = 6;
  bytes data = 7;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "roundtrip.SimpleMsg")
	if err != nil {
		t.Fatalf("failed to resolve message: %v", err)
	}

	inputJSON := []byte(`{"name":"hello","count":42,"bigCount":"999999999999","ratio":3.14,"precise":2.718281828,"active":true,"data":"aGVsbG8="}`)

	// JSON → binary.
	binData, err := reg.ProtoJSONToBytes(msgDesc, inputJSON)
	if err != nil {
		t.Fatalf("ProtoJSONToBytes failed: %v", err)
	}
	if len(binData) == 0 {
		t.Fatal("expected non-empty binary output")
	}

	// binary → JSON.
	outputJSON, err := reg.BytesToProtoJSON(msgDesc, binData)
	if err != nil {
		t.Fatalf("BytesToProtoJSON failed: %v", err)
	}

	// Verify semantic equivalence by comparing parsed JSON.
	var inputMap, outputMap map[string]interface{}
	if err := json.Unmarshal(inputJSON, &inputMap); err != nil {
		t.Fatalf("failed to parse input JSON: %v", err)
	}
	if err := json.Unmarshal(outputJSON, &outputMap); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}

	// Check key fields are preserved.
	if outputMap["name"] != "hello" {
		t.Errorf("name: want hello, got %v", outputMap["name"])
	}
	if outputMap["active"] != true {
		t.Errorf("active: want true, got %v", outputMap["active"])
	}
}

func TestProtoJSONToBytes_RoundTrip_EnumField(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package enumtest;

enum Status {
  UNKNOWN = 0;
  ACTIVE = 1;
  INACTIVE = 2;
}

message StatusMsg {
  Status status = 1;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "enumtest.StatusMsg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	inputJSON := []byte(`{"status":"ACTIVE"}`)
	binData, err := reg.ProtoJSONToBytes(msgDesc, inputJSON)
	if err != nil {
		t.Fatalf("ProtoJSONToBytes failed: %v", err)
	}

	outputJSON, err := reg.BytesToProtoJSON(msgDesc, binData)
	if err != nil {
		t.Fatalf("BytesToProtoJSON failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(outputJSON, &out); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if out["status"] != "ACTIVE" {
		t.Errorf("enum round-trip: want ACTIVE, got %v", out["status"])
	}
}

func TestProtoJSONToBytes_RoundTrip_NestedMessage(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package nestedrt;

message Address {
  string street = 1;
  string city = 2;
}

message Person {
  string name = 1;
  Address address = 2;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "nestedrt.Person")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	inputJSON := []byte(`{"name":"Alice","address":{"street":"123 Main St","city":"Springfield"}}`)
	binData, err := reg.ProtoJSONToBytes(msgDesc, inputJSON)
	if err != nil {
		t.Fatalf("ProtoJSONToBytes failed: %v", err)
	}

	outputJSON, err := reg.BytesToProtoJSON(msgDesc, binData)
	if err != nil {
		t.Fatalf("BytesToProtoJSON failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(outputJSON, &out); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if out["name"] != "Alice" {
		t.Errorf("name: want Alice, got %v", out["name"])
	}
	addr, ok := out["address"].(map[string]interface{})
	if !ok {
		t.Fatal("address should be a nested object")
	}
	if addr["street"] != "123 Main St" {
		t.Errorf("street: want '123 Main St', got %v", addr["street"])
	}
}

func TestProtoJSONToBytes_RoundTrip_RepeatedAndMap(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package collections;

message CollMsg {
  repeated string tags = 1;
  map<string, int32> scores = 2;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "collections.CollMsg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	inputJSON := []byte(`{"tags":["a","b","c"],"scores":{"alice":100,"bob":95}}`)
	binData, err := reg.ProtoJSONToBytes(msgDesc, inputJSON)
	if err != nil {
		t.Fatalf("ProtoJSONToBytes failed: %v", err)
	}

	outputJSON, err := reg.BytesToProtoJSON(msgDesc, binData)
	if err != nil {
		t.Fatalf("BytesToProtoJSON failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(outputJSON, &out); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	tags, ok := out["tags"].([]interface{})
	if !ok {
		t.Fatal("tags should be an array")
	}
	if len(tags) != 3 {
		t.Errorf("tags: want 3 elements, got %d", len(tags))
	}

	scores, ok := out["scores"].(map[string]interface{})
	if !ok {
		t.Fatal("scores should be a map")
	}
	if len(scores) != 2 {
		t.Errorf("scores: want 2 entries, got %d", len(scores))
	}
}

func TestProtoJSONToBytes_UnknownField(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package unknown;

message Msg {
  string name = 1;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "unknown.Msg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	// JSON with an unknown field.
	inputJSON := []byte(`{"name":"test","nonexistent_field":"value"}`)
	_, err = reg.ProtoJSONToBytes(msgDesc, inputJSON)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestProtoJSONToBytes_TypeMismatch(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package mismatch;

message Msg {
  int32 count = 1;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "mismatch.Msg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	// Provide a string where int32 is expected.
	inputJSON := []byte(`{"count":"not_a_number"}`)
	_, err = reg.ProtoJSONToBytes(msgDesc, inputJSON)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

func TestProtoJSONToBytes_NilDescriptor(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.ProtoJSONToBytes(nil, []byte(`{"name":"test"}`))
	if err == nil {
		t.Fatal("expected error for nil descriptor")
	}
}

func TestProtoJSONToBytes_EmptyPayload(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package ep;

message Msg {
  string name = 1;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "ep.Msg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	_, err = reg.ProtoJSONToBytes(msgDesc, nil)
	if err == nil {
		t.Fatal("expected error for nil payload")
	}

	_, err = reg.ProtoJSONToBytes(msgDesc, []byte{})
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestBytesToProtoJSON_NilDescriptor(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.BytesToProtoJSON(nil, []byte{0x0a, 0x04, 0x74, 0x65, 0x73, 0x74})
	if err == nil {
		t.Fatal("expected error for nil descriptor")
	}
}

func TestBytesToProtoJSON_EmptyData(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package ed;

message Msg {
  string name = 1;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "ed.Msg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	_, err = reg.BytesToProtoJSON(msgDesc, nil)
	if err == nil {
		t.Fatal("expected error for nil data")
	}

	_, err = reg.BytesToProtoJSON(msgDesc, []byte{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

// --- GenerateTemplate Tests ---

func TestGenerateTemplate_ScalarFields(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package tmpl;

message ScalarMsg {
  string name = 1;
  int32 count = 2;
  int64 big = 3;
  float ratio = 4;
  double precise = 5;
  bool active = 6;
  bytes data = 7;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "tmpl.ScalarMsg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	tmpl, err := reg.GenerateTemplate(msgDesc)
	if err != nil {
		t.Fatalf("GenerateTemplate failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(tmpl, &out); err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	// Scalar zero values.
	if out["name"] != "" {
		t.Errorf("name: want empty string, got %v", out["name"])
	}
	if out["active"] != false {
		t.Errorf("active: want false, got %v", out["active"])
	}
	// Numeric zeros — JSON numbers are float64.
	if count, ok := out["count"].(float64); !ok || count != 0 {
		t.Errorf("count: want 0, got %v", out["count"])
	}
}

func TestGenerateTemplate_NestedMessage(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package tmpl;

message Inner {
  string value = 1;
}

message Outer {
  string name = 1;
  Inner nested = 2;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "tmpl.Outer")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	tmpl, err := reg.GenerateTemplate(msgDesc)
	if err != nil {
		t.Fatalf("GenerateTemplate failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(tmpl, &out); err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	nested, ok := out["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("nested should be an object ({})")
	}
	// The nested message should be present (possibly with zero fields emitted).
	_ = nested
}

func TestGenerateTemplate_RepeatedFields(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package tmpl;

message RepMsg {
  string name = 1;
  repeated string tags = 2;
  repeated int32 numbers = 3;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "tmpl.RepMsg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	tmpl, err := reg.GenerateTemplate(msgDesc)
	if err != nil {
		t.Fatalf("GenerateTemplate failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(tmpl, &out); err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	tags, ok := out["tags"].([]interface{})
	if !ok {
		t.Fatal("tags should be an array")
	}
	if len(tags) != 0 {
		t.Errorf("tags: want empty array, got %v", tags)
	}

	numbers, ok := out["numbers"].([]interface{})
	if !ok {
		t.Fatal("numbers should be an array")
	}
	if len(numbers) != 0 {
		t.Errorf("numbers: want empty array, got %v", numbers)
	}
}

func TestGenerateTemplate_MapFields(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package tmpl;

message MapMsg {
  string name = 1;
  map<string, int32> scores = 2;
  map<int32, string> labels = 3;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "tmpl.MapMsg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	tmpl, err := reg.GenerateTemplate(msgDesc)
	if err != nil {
		t.Fatalf("GenerateTemplate failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(tmpl, &out); err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	scores, ok := out["scores"].(map[string]interface{})
	if !ok {
		t.Fatal("scores should be an object ({})")
	}
	if len(scores) != 0 {
		t.Errorf("scores: want empty map, got %v", scores)
	}

	labels, ok := out["labels"].(map[string]interface{})
	if !ok {
		t.Fatal("labels should be an object ({})")
	}
	if len(labels) != 0 {
		t.Errorf("labels: want empty map, got %v", labels)
	}
}

func TestGenerateTemplate_OneofField(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package tmpl;

message OneofMsg {
  string common = 1;
  oneof choice {
    string text_val = 2;
    int32 int_val = 3;
  }
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "tmpl.OneofMsg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	tmpl, err := reg.GenerateTemplate(msgDesc)
	if err != nil {
		t.Fatalf("GenerateTemplate failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(tmpl, &out); err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	// Only the first oneof option should be present (text_val).
	if _, ok := out["textVal"]; !ok {
		// Also accept snake_case depending on protojson output.
		if _, ok := out["text_val"]; !ok {
			t.Error("expected first oneof option (textVal or text_val) to be present")
		}
	}

	// The second oneof option should NOT be present.
	if _, ok := out["intVal"]; ok {
		t.Error("second oneof option (intVal) should not be present")
	}
	if _, ok := out["int_val"]; ok {
		t.Error("second oneof option (int_val) should not be present")
	}
}

func TestGenerateTemplate_EnumField(t *testing.T) {
	fds := buildTestFDS(t, `
syntax = "proto3";
package tmpl;

enum Color {
  RED = 0;
  GREEN = 1;
  BLUE = 2;
}

message EnumMsg {
  Color color = 1;
  string label = 2;
}
`)
	reg := NewRegistry()
	msgDesc, err := ResolveMessageDescriptor(fds, "tmpl.EnumMsg")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	tmpl, err := reg.GenerateTemplate(msgDesc)
	if err != nil {
		t.Fatalf("GenerateTemplate failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(tmpl, &out); err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	// Proto3 enum zero value is the first declared value (RED = 0).
	// protojson with EmitUnpopulated emits the zero enum as its name or numeric value.
	// The zero enum value may be emitted as "RED" or as 0.
	colorVal, exists := out["color"]
	if !exists {
		t.Fatal("expected 'color' field in template")
	}
	// Zero enum value — protojson EmitUnpopulated emits it as the enum name string.
	switch v := colorVal.(type) {
	case string:
		if v != "RED" {
			t.Errorf("color: want RED (first enum value), got %s", v)
		}
	case float64:
		if v != 0 {
			t.Errorf("color: want 0 (first enum value), got %v", v)
		}
	default:
		t.Errorf("color: unexpected type %T", colorVal)
	}
}

func TestGenerateTemplate_NilDescriptor(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.GenerateTemplate(nil)
	if err == nil {
		t.Fatal("expected error for nil descriptor")
	}
}
