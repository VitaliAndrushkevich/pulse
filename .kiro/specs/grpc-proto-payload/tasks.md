# Implementation Plan: gRPC Proto Payload

## Overview

This plan implements the grpcurl-style proto source support for gRPC monitors. It replaces the raw base64 payload textarea with a schema-aware system supporting proto file upload, Server Reflection, and Proto JSON input with a CodeMirror-based editor featuring autocompletion and validation.

## Tasks

### Phase 1: Database & Data Layer

- [ ] 1. Create database migration for proto_sources table
  - Create `backend/migrations/012_proto_sources.up.sql` with `proto_sources` table (UUID PK, monitor_id UNIQUE FK with CASCADE DELETE, source_type CHECK constraint for 'upload'|'reflection', descriptor_bytes BYTEA, metadata JSONB, created_at/updated_at timestamps)
  - Create `backend/migrations/012_proto_sources.down.sql` to drop the table
  - Requirements: 8.1, 8.2, 8.3

- [ ] 2. Create sqlc queries for proto_sources CRUD
  - Add `backend/internal/store/postgres/queries/proto_sources.sql` with UpsertProtoSource (:one), GetProtoSource (:one), DeleteProtoSource (:exec), ProtoSourceExists (:one)
  - Run `sqlc generate` and verify generated Go code compiles
  - Requirements: 8.1, 8.5

### Phase 2: Schema Registry Core

- [ ] 3. Create proto registry package with FileDescriptorSet parsing
  - Create `backend/internal/proto/registry.go` with `Registry` struct
  - Implement `ParseFileDescriptorSet` — validates binary FileDescriptorSet, returns parsed `*descriptorpb.FileDescriptorSet`
  - Implement `ParseProtoFiles` — parses raw `.proto` file contents, detects unresolved imports, returns FileDescriptorSet
  - Requirements: 1.1, 1.2, 1.3, 1.4

- [ ] 4. Implement metadata extraction and message resolution
  - Implement `ExtractMetadata` — walks FileDescriptorSet, extracts services, methods, input/output types, field definitions with comments
  - Implement `ResolveMessageDescriptor` — finds message descriptor by fully-qualified name using `protodesc` and `protoregistry`
  - Create `backend/internal/proto/types.go` with ProtoSourceMetadata, ProtoService, ProtoMethod, ProtoField structs
  - Requirements: 1.6, 4.5, 3.1, 7.1

- [ ] 5. Implement Proto JSON ↔ binary conversion and template generation
  - Implement `ProtoJSONToBytes` — converts Proto JSON to binary protobuf using `protojson.Unmarshal` + `proto.Marshal` with `dynamicpb.Message`
  - Implement `BytesToProtoJSON` — converts binary to Proto JSON using `proto.Unmarshal` + `protojson.Marshal`
  - Implement `GenerateTemplate` — generates Proto JSON template with zero-value defaults for all field types
  - Requirements: 3.1, 7.1, 7.2, 7.5, 7.6, 7.7, 4.1

- [ ] 6. Write unit tests for Schema Registry
  - Test ParseFileDescriptorSet with valid and invalid binary input
  - Test ParseProtoFiles with valid protos, unresolved imports, and invalid syntax
  - Test ExtractMetadata completeness (all services, methods, types extracted)
  - Test ProtoJSONToBytes and BytesToProtoJSON with various field types, error cases
  - Test GenerateTemplate for scalars, nested messages, repeated, map, oneof
  - Requirements: 1.3, 1.4, 7.1–7.7

### Phase 3: Server Reflection Client

- [ ] 7. Implement Server Reflection discovery
  - Create `backend/internal/proto/reflect.go` with `ReflectServices` function
  - Connect to gRPC server using provided TLS config, call reflection ListServices
  - Fetch FileDescriptors for each service with transitive dependencies
  - Assemble complete FileDescriptorSet, enforce 10s timeout
  - Handle errors: no reflection support, timeout, unreachable, no services found
  - Requirements: 2.1–2.7

- [ ] 8. Write tests for Server Reflection client
  - Create test gRPC server with reflection enabled, verify full schema discovery
  - Test negative cases: server without reflection, timeout, unreachable target, server with only reflection service
  - Requirements: 2.3, 2.4, 2.6, 2.7

### Phase 4: API Handlers

- [ ] 9. Create proto source handler with Upload endpoint
  - Create `backend/internal/api/handlers/proto_source.go` with ProtoSourceHandler struct
  - Implement Upload handler: accepts multipart/form-data, validates size ≤ 5MB, detects file type (.proto vs .desc), calls appropriate Registry parse method, extracts metadata, upserts to DB, returns ProtoSourceMeta
  - Requirements: 1.1–1.6, 5.1, 8.4, 8.5

- [ ] 10. Implement Reflect, Get, and Delete handlers
  - Reflect handler: reads monitor target/TLS from DB, calls ReflectServices, validates non-empty services, upserts, returns metadata
  - Get handler: fetches by monitor_id, returns metadata or 404
  - Delete handler: deletes proto source, resets payload_format to "raw" if needed, returns 200 (idempotent)
  - Requirements: 2.1–2.7, 5.2–5.7

- [ ] 11. Register proto source routes in API router with auth middleware
  - Wire routes in `backend/internal/api/router.go` under authenticated group
  - Validate monitor_id exists middleware (return 404 for unknown monitors)
  - Requirements: 5.1–5.7

- [ ] 12. Write API handler tests
  - Test upload valid .proto, upload .desc, upload invalid files, size exceeded
  - Test reflect success, reflect failures (no reflection, timeout, unreachable)
  - Test get success, get 404, delete, delete idempotent, monitor not found
  - Requirements: 5.1–5.7

### Phase 5: GRPCChecker Integration

- [ ] 13. Add payload_format support to GRPCChecker
  - Add `PayloadFormat` field to `GRPCSettings` struct
  - Implement `resolvePayload` function: "raw" uses existing base64 decode, "proto_json" loads proto source from DB, resolves message descriptor, converts JSON to binary
  - Handle error cases: proto_json without proto source, invalid JSON, payload too large
  - Requirements: 3.1, 3.3, 3.5–3.7

- [ ] 14. Integrate resolvePayload into GRPCChecker.Check
  - Replace direct validateRequestPayload call with resolvePayload
  - Proto_json validation errors report monitor as "down" with descriptive error message
  - Ensure backward compatibility: "raw" or absent payload_format works exactly as before
  - Requirements: 3.1–3.7

- [ ] 15. Write GRPCChecker integration tests
  - Test proto_json with valid schema and payload, invalid JSON, missing proto source
  - Test raw format unchanged behavior
  - Test payload size exceeded for both formats
  - Requirements: 3.1–3.7

### Phase 6: OpenAPI Spec Update

- [ ] 16. Update OpenAPI specification
  - Add proto-source endpoints (POST upload, POST reflect, GET, DELETE) to `backend/api/openapi.yaml`
  - Add request/response schemas: ProtoSourceMeta, ProtoService, ProtoMethod, ProtoMessageSchema, ProtoField
  - Add error codes: PROTO_PARSE_ERROR, PROTO_UNRESOLVED_IMPORTS, PROTO_SIZE_EXCEEDED, etc.
  - Add `payload_format` field to GrpcMonitorSettings schema
  - Requirements: 5.1–5.7, 3.3

### Phase 7: Frontend Types & API Client

- [ ] 17. Add frontend TypeScript types for proto source
  - Add ProtoSourceMeta, ProtoService, ProtoMethod, ProtoMessageSchema, ProtoField interfaces to `frontend/src/lib/types.ts`
  - Add `payload_format?: 'raw' | 'proto_json'` to GrpcSettings interface
  - Requirements: 3.3, 5.1–5.4

- [ ] 18. Add proto source API client functions
  - Add to `frontend/src/lib/api.ts`: uploadProtoSource (multipart), triggerReflection, getProtoSource, deleteProtoSource
  - Requirements: 5.1–5.5, 6.1–6.9

### Phase 8: Frontend Components

- [ ] 19. Create ProtoSourceUpload component
  - File drop zone accepting .proto and .desc files (max 20 files)
  - "Use Server Reflection" button (enabled when target is set, disabled with hint otherwise)
  - Progress indicator during upload/reflection
  - Display current source info if exists (source type, filenames, services) with replace/remove options
  - Error display for upload/reflection failures
  - Requirements: 6.1, 6.2, 6.5–6.10

- [ ] 20. Add service/method selector to ProtoSourceUpload
  - After successful upload/reflection, show discovered services and methods
  - User selects exactly one service method
  - On confirm, emit selection event with chosen method and proto source metadata
  - Requirements: 6.3, 6.4, 6.8

- [ ] 21. Install CodeMirror 6 dependencies and create PayloadEditor component
  - Install via pnpm: `codemirror`, `@codemirror/lang-json`, `@codemirror/autocomplete`, `@codemirror/lint`
  - Create `frontend/src/components/PayloadEditor.svelte` with CodeMirror 6 + JSON mode
  - Accept optional schema prop; fall back to plain textarea when no schema
  - Requirements: 4.1, 4.6, 4.7

- [ ] 22. Implement schema autocompletion extension
  - Create `frontend/src/lib/codemirror/schema-autocomplete.ts`
  - Field name suggestions based on message schema context
  - Enum value suggestions when cursor is at enum field value
  - Debounced 200ms, max 20 suggestions
  - Requirements: 4.2, 4.4

- [ ] 23. Implement schema validation and tooltip extensions
  - Create `frontend/src/lib/codemirror/schema-lint.ts` — inline error markers for unknown fields, type mismatches, debounced 500ms
  - Create `frontend/src/lib/codemirror/schema-tooltip.ts` — shows proto field comments on hover, truncated 256 chars
  - Requirements: 4.3, 4.5

- [ ] 24. Integrate new components into GrpcSettingsForm
  - Add payload_format toggle (raw / proto_json) to form
  - Integrate ProtoSourceUpload component
  - Replace textarea with PayloadEditor, pass schema metadata from proto source
  - Connect upload results to service_method and schema state
  - Requirements: 4.6, 4.7, 6.3, 6.4

### Phase 9: Internationalization

- [ ] 25. Add i18n keys for all new UI strings
  - Add to `frontend/src/locales/en.json`: proto source upload labels, reflection button text, payload format toggle, error messages, editor placeholders, service/method selector labels
  - Propagate keys to all 12 other locale files (ar, be, de, es, fr, it, ja, ko, pt, ru, tr, zh) with English placeholders
  - Run locale validation script
  - Requirements: (AGENTS.md i18n mandate)

### Phase 10: Frontend Tests

- [ ] 26. Write ProtoSourceUpload component tests
  - Test file upload flow (success, error), reflection flow (success, error)
  - Test button enable/disable based on target availability
  - Test existing source display and replace/remove actions
  - Requirements: 6.1–6.10

- [ ] 27. Write PayloadEditor component tests
  - Test schema-aware mode with autocompletion and validation
  - Test textarea fallback when no schema
  - Test error state when schema parse fails
  - Requirements: 4.1–4.7

### Phase 11: Property-Based Tests

- [ ] 28. Write backend property-based tests (rapid)
  - Proto JSON round-trip (Property 5): serialize → deserialize produces equivalent output
  - Metadata completeness (Property 4): all services/methods/types extracted
  - Template generation (Property 8): all fields present with correct zero values
  - Invalid content rejection (Property 3): arbitrary bytes rejected with error
  - Requirements: 7.1–7.7, 1.4, 1.6

- [ ] 29. Write frontend property-based tests (fast-check)
  - Schema-to-completion-items mapping: every field in schema produces completion item
  - Schema-to-validation logic: conforming JSON passes, non-conforming fails with correct field
  - Requirements: 4.2, 4.3

## Task Dependency Graph

```json
{
  "waves": [
    [1],
    [2],
    [3, 4, 5],
    [6, 7],
    [8, 9, 10],
    [11, 12],
    [13, 14],
    [15, 16],
    [17, 18],
    [19, 20, 21],
    [22, 23, 25],
    [24],
    [26, 27],
    [28, 29]
  ]
}
```

Critical path: 1 → 2 → 3 → 5 → 9 → 10 → 11 → 13 → 14 → 16 → 17 → 18 → 19 → 21 → 22 → 23 → 24 → 25

Parallelizable:
- Tasks 7–8 (Reflection) can run in parallel with tasks 4–5 (after task 2)
- Tasks 17–18 (Frontend types) can start after task 16 (OpenAPI)
- Tasks 26–29 (Tests) can run after their respective implementation tasks

## Notes

- The `google.golang.org/protobuf` package is already a transitive dependency via `google.golang.org/grpc` — no new Go dependency needed for proto manipulation
- For `.proto` file parsing without invoking `protoc`, use `github.com/bufbuild/protocompile` which provides a pure-Go protobuf compiler
- CodeMirror 6 is a new frontend dependency (~50KB gzipped) — acceptable for this feature's editor needs
- Backward compatibility is maintained: existing monitors with base64 payloads continue to work unchanged (payload_format defaults to "raw")
- The GRPCChecker will need access to the database queries to load proto sources during check execution — this requires passing the queries instance through the checker constructor or the scheduler context
