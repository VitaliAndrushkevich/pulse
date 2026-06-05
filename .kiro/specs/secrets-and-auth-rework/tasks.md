# Implementation Plan: Secrets and Auth Rework

## Overview

This plan implements the separation of the monolithic "secrets" concept into Monitor Credentials (per-monitor encrypted auth for health checks) and API Tokens (programmatic Pulse API access in Settings). Work is organized as: database migration → backend service layer → checker integration → OpenAPI spec → frontend components → cleanup of legacy code.

## Tasks

- [x] 1. Database migration and sqlc queries
  - [x] 1.1 Create migration 006_monitor_credentials
    - Create `backend/migrations/006_monitor_credentials.up.sql` with the `monitor_credentials` table (id UUID PK, monitor_id FK with ON DELETE CASCADE, auth_type TEXT CHECK, name TEXT, encrypted_value TEXT, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ) and index on monitor_id
    - Create `backend/migrations/006_monitor_credentials.down.sql` that drops the table
    - _Requirements: 7.1, 7.2_

  - [x] 1.2 Add sqlc credential queries
    - Create `backend/internal/store/postgres/queries/credentials.sql` with all queries from the design: CreateCredential, ListCredentialsByMonitorID, GetCredential, UpdateCredential, DeleteCredential, ListCredentialsByMonitorIDInternal, ListAllCredentials, UpdateCredentialEncryptedValue
    - Run `sqlc generate` to produce Go code
    - _Requirements: 11.1, 11.2, 11.3, 11.4_

- [x] 2. Backend credential types and encryption helpers
  - [x] 2.1 Define credential domain types
    - Create `backend/internal/api/handlers/credential_types.go` with `CreateCredentialRequest`, `UpdateCredentialRequest`, `CredentialResponse`, `CredentialPayload`, and `AuthCredential` structs as specified in the design
    - _Requirements: 1.1, 1.2, 1.3, 2.3, 2.4_

  - [x] 2.2 Implement credential payload encryption/decryption helpers
    - Create helper functions `encryptCredentialPayload(key []byte, payload CredentialPayload) (string, error)` and `decryptCredentialPayload(key []byte, encrypted string) (CredentialPayload, error)` that serialize JSON then use existing `crypto.Encrypt`/`crypto.Decrypt`
    - Place in `backend/internal/api/handlers/credential_crypto.go`
    - _Requirements: 10.1, 10.5_

  - [x]* 2.3 Write property test: Credential encryption round-trip
    - **Property 1: Credential encryption round-trip**
    - **Validates: Requirements 1.1, 1.2, 1.3, 10.1**
    - Use `pgregory.net/rapid` to generate arbitrary CredentialPayload values and verify encrypt→decrypt produces identical output

- [x] 3. Backend credential handler (CRUD)
  - [x] 3.1 Implement CredentialHandler with Create endpoint
    - Create `backend/internal/api/handlers/credentials.go` with `CredentialHandler` struct, `Create` method that validates auth_type, required fields per type, encrypts payload, calls sqlc CreateCredential, returns metadata-only response
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.7, 1.8, 11.1_

  - [x] 3.2 Implement List and Get endpoints
    - Add `List` method returning credential metadata without secrets, including header_name for header type and username for basic type
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 11.2_

  - [x] 3.3 Implement Update endpoint
    - Add `Update` method that re-encrypts new secret values, updates timestamp, returns metadata only
    - _Requirements: 3.1, 3.2, 3.3, 11.3_

  - [x] 3.4 Implement Delete endpoint
    - Add `Delete` method that removes credential by id and monitor_id
    - _Requirements: 4.1, 4.3, 11.4_

  - [x] 3.5 Register credential routes in API router
    - Add routes in `backend/internal/api/router.go` under `/api/v1/monitors/:id/credentials` group with combined auth middleware
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6_

  - [x]* 3.6 Write property test: API responses never expose plaintext secrets
    - **Property 2: API responses never expose plaintext secrets**
    - **Validates: Requirements 1.4, 2.1, 10.4**
    - Generate arbitrary credentials, call handler methods, assert response JSON never contains token/password/header_value/encrypted_value fields

  - [x]* 3.7 Write property test: Metadata includes correct non-secret fields per auth_type
    - **Property 3: Metadata includes correct non-secret fields per auth_type**
    - **Validates: Requirements 2.3, 2.4**
    - For header type: response includes header_name. For basic type: response includes username but not password.

  - [x]* 3.8 Write property test: Invalid auth_type produces validation error
    - **Property 4: Invalid auth_type produces validation error**
    - **Validates: Requirements 1.7**
    - Generate arbitrary strings not in {"bearer","basic","header"}, assert 400 response

- [x] 4. Checkpoint - Backend CRUD verified
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Checker interface extension and credential injection
  - [x] 5.1 Define AuthenticatedChecker interface
    - Create `backend/internal/monitor/auth.go` with `AuthCredential` struct and `AuthenticatedChecker` interface extending `Checker` with `CheckWithAuth` method
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 6.1, 6.2, 6.3_

  - [x] 5.2 Implement AuthenticatedChecker in HTTP checker
    - Add `CheckWithAuth` method to the HTTP checker that injects Authorization headers (Bearer, Basic base64) and custom headers from credentials into outbound requests
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5_

  - [x] 5.3 Implement AuthenticatedChecker in WebSocket checker
    - Add `CheckWithAuth` method to the WebSocket checker that injects auth headers into the upgrade request
    - _Requirements: 6.1, 6.2, 6.3, 6.4_

  - [x]* 5.4 Write property test: HTTP Checker injects all credentials correctly
    - **Property 5: HTTP Checker injects all credentials correctly**
    - **Validates: Requirements 5.1, 5.2, 5.3, 5.4**
    - Generate arbitrary credential sets, use httptest server to capture headers, verify correct injection

  - [x]* 5.5 Write property test: WebSocket Checker injects all credentials correctly
    - **Property 6: WebSocket Checker injects all credentials correctly**
    - **Validates: Requirements 6.1, 6.2, 6.3**
    - Generate arbitrary credential sets, use test WS server to capture upgrade headers, verify correct injection

- [x] 6. Scheduler credential loading
  - [x] 6.1 Extend scheduler executeCheck to load and decrypt credentials
    - Modify `backend/internal/monitor/scheduler.go` `executeCheck` method to load credentials for HTTP/WebSocket monitors, decrypt them, and call `CheckWithAuth` when credentials exist (fall back to `Check` otherwise)
    - Handle decryption failures by reporting monitor as down with credential error
    - _Requirements: 5.5, 6.4, 10.5_

  - [x]* 6.2 Write property test: Credential update replaces encrypted value
    - **Property 7: Credential update replaces encrypted value**
    - **Validates: Requirements 3.1, 3.2**
    - Generate old and new payloads, update credential, decrypt stored value and assert it matches new value with updated timestamp

  - [x]* 6.3 Write property test: Credential deletion removes from listing
    - **Property 8: Credential deletion removes from listing**
    - **Validates: Requirements 4.1**
    - Create N credentials, delete one, list and assert count is N-1 and deleted ID is absent

- [x] 7. Key rotation extension
  - [x] 7.1 Extend key rotation to re-encrypt monitor_credentials
    - Modify the existing key rotation logic (used by `make rotate-key`) to also load all `monitor_credentials` rows, decrypt with old key, re-encrypt with new key, and update in a single transaction
    - _Requirements: 10.2_

- [x] 8. Checkpoint - Backend complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 9. OpenAPI spec update
  - [x] 9.1 Add credential endpoints and schemas to OpenAPI
    - Add paths `POST/GET /monitors/{id}/credentials` and `PUT/DELETE /monitors/{id}/credentials/{credentialId}` to `backend/api/openapi.yaml`
    - Add schemas: `MonitorCredential`, `CreateMonitorCredentialRequest`, `UpdateMonitorCredentialRequest`, `MonitorCredentialListResponse`
    - _Requirements: 11.1, 11.2, 11.3, 11.4_

- [x] 10. Frontend API client and types
  - [x] 10.1 Add credential API functions and types
    - Add `Credential`, `CreateCredentialRequest` interfaces and `createCredential`, `listCredentials`, `updateCredential`, `deleteCredential` async functions to `frontend/src/lib/api.ts`
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 12.1_

- [x] 11. Frontend ShowOnceModal component
  - [x] 11.1 Create ShowOnceModal component
    - Create `frontend/src/components/ShowOnceModal.svelte` — reusable modal with read-only secret display, copy-to-clipboard button with "Copied!" confirmation (2s), dismiss warning, and state cleanup on close
    - Must prevent navigation away without explicit dismissal
    - _Requirements: 13.1, 13.2, 13.3, 13.4, 13.5, 13.6_

  - [x]* 11.2 Write unit tests for ShowOnceModal
    - Test: modal renders secret value, copy button works, warning displayed, state clears on dismiss
    - _Requirements: 13.1, 13.2, 13.3, 13.4_

- [x] 12. Frontend credential management components
  - [x] 12.1 Create CredentialForm component
    - Create `frontend/src/components/CredentialForm.svelte` with auth_type selector (Bearer Token, Basic Auth, Custom Header) and appropriate input fields per type
    - _Requirements: 12.2_

  - [x] 12.2 Create CredentialList component
    - Create `frontend/src/components/CredentialList.svelte` displaying credential metadata (auth_type, name, created_at) with delete and replace actions, no secret reveal mechanism
    - _Requirements: 12.4, 12.5_

  - [x] 12.3 Create AuthSection container component
    - Create `frontend/src/components/AuthSection.svelte` that wraps CredentialForm and CredentialList within the monitor form, integrates ShowOnceModal on credential creation
    - _Requirements: 12.1, 12.3, 12.6_

  - [x]* 12.4 Write unit tests for credential management components
    - Test CredentialForm validation, CredentialList rendering, AuthSection integration, show-once flow
    - _Requirements: 12.1, 12.2, 12.4, 12.5_

- [x] 13. Integrate AuthSection into monitor form
  - [x] 13.1 Add AuthSection to monitor create/edit pages
    - Integrate AuthSection into the monitor form, visible only for HTTP and WebSocket monitor types, hidden for TCP/UDP
    - _Requirements: 12.1, 12.7_

- [x] 14. Frontend Settings page rework
  - [x] 14.1 Create ApiTokenSection component and update Settings page
    - Create `frontend/src/routes/settings/ApiTokenSection.svelte` with clear "API Tokens" labeling and description explaining programmatic API access
    - Remove legacy "Secrets" section from Settings page
    - Integrate ShowOnceModal for new token creation flow
    - Display token metadata only (name, created_at, last_used, expiration), never token values
    - Include revoke functionality
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 9.1_

  - [x]* 14.2 Write unit tests for ApiTokenSection
    - Test: token list rendering, create flow with show-once modal, revoke action, no secret display, correct labeling
    - _Requirements: 8.1, 8.2, 8.5, 8.6_

- [x] 15. Checkpoint - Frontend complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 16. Legacy cleanup and final integration
  - [x] 16.1 Remove legacy secrets endpoints and deprecate secrets table
    - Remove `/api/v1/secrets` routes from the router
    - Update OpenAPI spec to remove legacy secrets endpoints
    - Add deprecation comment to secrets table (future migration 007 will drop it)
    - _Requirements: 9.1, 9.2, 9.3_

  - [x]* 16.2 Write integration tests for full credential flow
    - Test monitor deletion cascades to credentials, key rotation re-encrypts credentials, full check flow with credentials injected
    - _Requirements: 7.1, 7.2, 10.2, 5.1_

- [x] 17. Final checkpoint - All tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document using `pgregory.net/rapid` (Go) and `fast-check` (TypeScript)
- Unit tests validate specific examples and edge cases
- The backend Go code uses existing patterns: sqlc queries, `crypto.Encrypt`/`Decrypt`, gin handlers
- The frontend uses Svelte 5, TypeScript strict, Tailwind CSS, and existing api.ts patterns
- OpenAPI spec must be updated in the same commit as handler changes per project conventions

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["2.1", "2.2"] },
    { "id": 3, "tasks": ["2.3", "3.1"] },
    { "id": 4, "tasks": ["3.2", "3.3", "3.4"] },
    { "id": 5, "tasks": ["3.5", "3.6", "3.7", "3.8"] },
    { "id": 6, "tasks": ["5.1"] },
    { "id": 7, "tasks": ["5.2", "5.3"] },
    { "id": 8, "tasks": ["5.4", "5.5", "6.1"] },
    { "id": 9, "tasks": ["6.2", "6.3", "7.1"] },
    { "id": 10, "tasks": ["9.1", "10.1"] },
    { "id": 11, "tasks": ["11.1"] },
    { "id": 12, "tasks": ["11.2", "12.1", "12.2"] },
    { "id": 13, "tasks": ["12.3", "14.1"] },
    { "id": 14, "tasks": ["12.4", "13.1", "14.2"] },
    { "id": 15, "tasks": ["16.1"] },
    { "id": 16, "tasks": ["16.2"] }
  ]
}
```
