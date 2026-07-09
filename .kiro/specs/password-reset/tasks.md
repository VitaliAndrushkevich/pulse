# Implementation Plan: Password Reset

## Overview

Implements two password management mechanisms: (1) env-var-driven admin reset mode that re-enables the setup flow to overwrite existing credentials, and (2) an authenticated password change endpoint with a Settings UI form. Includes SQL query additions, locale updates for 13 languages, OpenAPI documentation, and .env.example updates.

## Tasks

- [x] 1. Backend data layer and config
  - [x] 1.1 Add `ResetAdmin` field to config and parse `PULSE_RESET_ADMIN` env var
    - Add `ResetAdmin bool` field to the `App` struct in `backend/internal/config/config.go`
    - Parse using `strings.EqualFold(getEnv("PULSE_RESET_ADMIN", ""), "true")`
    - _Requirements: 1.1, 1.2_

  - [x] 1.2 Add new SQL queries and regenerate sqlc
    - Add `UpdateUserEmailAndPassword` query (`:one`, UPDATE with RETURNING *) to `backend/internal/store/postgres/queries/users.sql`
    - Add `GetFirstUser` query (`:one`, SELECT * FROM users LIMIT 1) to the same file
    - Run `sqlc generate` from `backend/` directory to regenerate Go code
    - _Requirements: 8.1, 8.2, 8.3, 8.4_

  - [x] 1.3 Write property tests for SQL query atomicity
    - **Property 2: Atomic Overwrite**
    - **Validates: Requirements 8.1, 8.2**

- [x] 2. Admin reset mode backend logic
  - [x] 2.1 Add startup warning log for admin reset mode
    - In `backend/cmd/pulse/main.go`, after `config.LoadApp()` and before HTTP server start, log the warning message when `cfg.ResetAdmin` is true
    - Pass `ResetAdmin` to the `Deps` struct in `backend/internal/api/router.go`
    - _Requirements: 3.1, 3.2, 3.3_

  - [x] 2.2 Modify SetupHandler to accept and use `resetAdmin` flag
    - Add `resetAdmin bool` field to `SetupHandler` struct in `backend/internal/api/handlers/setup.go`
    - Update `NewSetupHandler` constructor to accept the `resetAdmin` parameter
    - Update `Status` method: when `resetAdmin` is true, always return `setup_required: true`
    - _Requirements: 1.3, 1.4_

  - [x] 2.3 Implement admin reset overwrite logic in Setup handler
    - When `resetAdmin` is true and a user exists: fetch user via `GetFirstUser`, call `UpdateUserEmailAndPassword`, return JWT with HTTP 200
    - When `resetAdmin` is true and no user exists: create new user (existing logic), return JWT with HTTP 201
    - When `resetAdmin` is false and a user exists: return 409 with `SETUP_ALREADY_COMPLETE`
    - Validate email/password (min 8 chars) before any write, return 400 on failure
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_

  - [x] 2.4 Write unit tests for SetupHandler in admin reset mode
    - Test overwrite path (200 response, JWT returned, user preserved)
    - Test create-new path (201 response)
    - Test validation failure (400 with missing fields, short password)
    - Test 409 when reset mode is off and user exists
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6_

  - [x] 2.5 Write property test for FK integrity preservation
    - **Property 1: FK Integrity**
    - **Validates: Requirements 2.1, 2.2**

- [x] 3. Checkpoint - Ensure admin reset mode tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Authenticated password change backend
  - [x] 4.1 Add `ChangePassword` handler to AuthHandler
    - Add `ChangePassword` method to `backend/internal/api/handlers/auth.go`
    - Validate request body (require `current_password` and `new_password` string fields)
    - Validate `new_password` length: min 8 characters, max 72 bytes
    - Verify current password with bcrypt.CompareHashAndPassword
    - Hash new password with bcrypt.DefaultCost, call existing `UpdateUserPassword` query
    - Return 200 with `{"message": "password updated successfully"}` on success
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_

  - [x] 4.2 Register `PUT /auth/password` route on protected group
    - In `backend/internal/api/router.go`, add `protected.PUT("/auth/password", authHandler.ChangePassword)`
    - _Requirements: 4.1_

  - [x] 4.3 Write unit tests for ChangePassword handler
    - Test happy path (correct current password, valid new password → 200)
    - Test wrong current password → 401
    - Test new password too short (< 8 chars) → 400
    - Test new password too long (> 72 bytes) → 400
    - Test missing/invalid JSON body → 400
    - _Requirements: 4.2, 4.3, 4.4, 4.5, 4.7_

  - [x] 4.4 Write property test for bcrypt 72-byte safety
    - **Property 4: Bcrypt Safety**
    - **Validates: Requirements 4.4**

- [x] 5. Checkpoint - Ensure all backend tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Frontend password change UI
  - [x] 6.1 Add `changePassword` function to the API client
    - In `frontend/src/lib/api.ts`, add `changePassword(data: ChangePasswordRequest)` function
    - Use existing `apiRequest` helper with `PUT` method to `/auth/password`
    - Pass `{ skipToast: true }` to handle errors inline
    - _Requirements: 4.1_

  - [x] 6.2 Create `PasswordChangeSection.svelte` component
    - Create `frontend/src/routes/settings/PasswordChangeSection.svelte`
    - Three password fields: current, new, confirm (type="password", maxlength=128)
    - Disable submit until all fields have non-whitespace content and new === confirm
    - Inline validation: show min-length message when new password < 8 chars and field is dirty
    - Loading state: disable submit button and show indicator during API call
    - On success: show toast, clear all fields
    - On error: show API error message inline above form, preserve field values
    - Use `t()` for all user-visible strings under `settings.password.*` namespace
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7_

  - [x] 6.3 Integrate PasswordChangeSection into settings page
    - Import and render `<PasswordChangeSection />` in `frontend/src/routes/settings/+page.svelte`
    - _Requirements: 5.1_

  - [x] 6.4 Write Vitest unit tests for PasswordChangeSection
    - Test form validation (disabled button, min-length, mismatch)
    - Test submit flow (loading state, success toast, field clearing)
    - Test error display (inline error, field preservation)
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_

- [x] 7. Localization updates
  - [x] 7.1 Add `settings.password.*` i18n keys to `en.json`
    - Add all password change section keys under `settings.password` namespace in `frontend/src/locales/en.json`
    - Include: title, description, field labels, placeholders, submit button, submitting state, success message, validation messages, error messages
    - _Requirements: 7.1, 7.2_

  - [x] 7.2 Propagate i18n keys to all 12 non-English locale files
    - Copy the English `settings.password.*` keys to ar, be, de, es, fr, it, ja, ko, pt, ru, tr, zh locale files as English placeholders
    - _Requirements: 7.2, 7.3_

  - [x] 7.3 Verify locale completeness
    - Run `pnpm --filter frontend run validate-locales` and confirm zero errors
    - _Requirements: 7.4_

- [x] 8. Checkpoint - Ensure frontend tests and locale validation pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 9. Documentation updates
  - [x] 9.1 Add `PULSE_RESET_ADMIN` to `.env.example`
    - Add commented-out variable with inline description of purpose, accepted values, default behavior, and usage example
    - Follow existing comment style in the file
    - _Requirements: 6.1_

  - [x] 9.2 Update OpenAPI spec with password change endpoint
    - Add `PUT /api/v1/auth/password` to `backend/api/openapi.yaml`
    - Include operationId, summary, tags, request body schema, 200 success response, 400 and 401 error responses
    - _Requirements: 6.2_

  - [x] 9.3 Update OpenAPI spec with admin reset mode documentation
    - Update `GET /api/v1/auth/setup` description to note Admin_Reset_Mode behavior
    - Update `POST /api/v1/auth/setup` to document overwrite behavior when reset mode active (200 for overwrite, 201 for new), and 400 validation error
    - _Requirements: 6.3, 6.4_

  - [x] 9.4 Add `PULSE_RESET_ADMIN` to AGENTS.md environment variables table
    - Add entry to the Infrastructure environment variables section
    - _Requirements: 6.1_

- [x] 10. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- No database migrations needed — only new sqlc queries added to existing schema
- The `sqlc generate` step in task 1.2 must complete before tasks 2.2/2.3 can use the generated Go functions
- Frontend already has redirect logic from `/login` to `/setup` when `setup_required: true` — no frontend changes needed for admin reset flow
- The project uses `make test` for backend Go tests and `pnpm test` (in `frontend/`) for frontend Vitest tests
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2"] },
    { "id": 1, "tasks": ["1.3", "2.1", "2.2", "6.1", "7.1"] },
    { "id": 2, "tasks": ["2.3", "4.1", "7.2"] },
    { "id": 3, "tasks": ["2.4", "2.5", "4.2", "7.3"] },
    { "id": 4, "tasks": ["4.3", "4.4", "6.2"] },
    { "id": 5, "tasks": ["6.3", "6.4"] },
    { "id": 6, "tasks": ["9.1", "9.2", "9.3", "9.4"] }
  ]
}
```
