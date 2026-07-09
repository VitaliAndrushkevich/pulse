# Requirements Document

## Introduction

Password reset functionality for Pulse, a single-user self-hosted uptime monitoring platform. This feature provides two mechanisms for credential recovery: an environment-variable-driven admin reset that re-enables the initial setup flow, and an authenticated password change endpoint accessible from the Settings UI. Additionally, documentation and locale completeness are addressed.

## Glossary

- **Setup_Handler**: The backend handler at `/api/v1/auth/setup` responsible for initial admin user creation and admin reset flow
- **Auth_Handler**: The backend handler responsible for authentication endpoints including login and password change
- **Admin_Reset_Mode**: A runtime mode activated by the `PULSE_RESET_ADMIN` environment variable that re-enables the setup flow for an existing instance
- **Password_Change_Endpoint**: The `PUT /api/v1/auth/password` authenticated endpoint for changing the current user password
- **Settings_UI**: The frontend settings page where authenticated users manage account preferences
- **Config_Loader**: The `config.LoadApp()` function that reads environment variables at startup

## Requirements

### Requirement 1: Admin Reset Mode Activation

**User Story:** As a self-hosted operator, I want to reset my admin credentials via an environment variable, so that I can regain access to my Pulse instance without direct database manipulation.

#### Acceptance Criteria

1. WHEN the `PULSE_RESET_ADMIN` environment variable is set to a case-insensitive variant of `true` (e.g., `true`, `TRUE`, `True`), THE Config_Loader SHALL include a `ResetAdmin` boolean field set to `true` in the application configuration
2. WHEN the `PULSE_RESET_ADMIN` environment variable is not set, is empty, or is set to any value that is not a case-insensitive match of `true`, THE Config_Loader SHALL set the `ResetAdmin` field to `false`
3. WHILE Admin_Reset_Mode is active, THE Setup_Handler SHALL report `setup_required: true` on `GET /api/v1/auth/setup` regardless of existing user count
4. WHILE Admin_Reset_Mode is active, THE Setup_Handler SHALL accept `POST /api/v1/auth/setup` requests regardless of existing user count

### Requirement 2: Admin Reset User Overwrite

**User Story:** As a self-hosted operator, I want the admin reset to overwrite my existing credentials rather than deleting the user, so that all my notification bindings, API tokens, and other associated data remain intact.

#### Acceptance Criteria

1. WHEN a `POST /api/v1/auth/setup` request is received while Admin_Reset_Mode is active and a user already exists, THE Setup_Handler SHALL update the existing user's email and password_hash fields in-place and set updated_at to the current timestamp, without deleting or recreating the user row
2. WHEN the existing user is overwritten during admin reset, THE Setup_Handler SHALL preserve the user UUID so that foreign key references in api_tokens and other tables referencing users.id remain valid
3. WHEN the credential overwrite completes successfully during Admin_Reset_Mode, THE Setup_Handler SHALL return a JWT token for the updated user with HTTP status 200
4. WHEN no user exists and Admin_Reset_Mode is active, THE Setup_Handler SHALL create a new user and return a JWT token with HTTP status 201
5. IF a `POST /api/v1/auth/setup` request is received while Admin_Reset_Mode is not active and a user already exists, THEN THE Setup_Handler SHALL reject the request with HTTP status 409 and error code SETUP_ALREADY_COMPLETE
6. IF the email or password field is missing or the password is shorter than 8 characters in a `POST /api/v1/auth/setup` request while Admin_Reset_Mode is active, THEN THE Setup_Handler SHALL reject the request with HTTP status 400 and error code VALIDATION_ERROR without modifying the existing user
7. IF the database update fails during admin reset credential overwrite, THEN THE Setup_Handler SHALL return HTTP status 500 with error code INTERNAL_ERROR and the existing user record SHALL remain unchanged

### Requirement 3: Admin Reset Startup Warning

**User Story:** As a self-hosted operator, I want Pulse to log a warning when admin reset mode is active, so that I remember to remove the environment variable after completing the reset.

#### Acceptance Criteria

1. WHEN the application starts with `PULSE_RESET_ADMIN` set to `true`, THE application SHALL log a message via `log.Printf` containing the text "WARNING: Admin reset mode is active. Remove PULSE_RESET_ADMIN after completing setup."
2. THE warning message SHALL be logged after configuration is loaded and before the HTTP server begins accepting connections, so it appears in the startup log sequence between the config phase and the listening announcement
3. IF `PULSE_RESET_ADMIN` is not set or is set to any value other than `true`, THEN THE application SHALL NOT log the admin reset warning message

### Requirement 4: Authenticated Password Change

**User Story:** As an authenticated user, I want to change my password from the Settings page, so that I can rotate my credentials without restarting the application.

#### Acceptance Criteria

1. THE Password_Change_Endpoint SHALL be accessible at `PUT /api/v1/auth/password` and require valid authentication (JWT or API token)
2. WHEN a valid request is received, THE Password_Change_Endpoint SHALL validate that the `current_password` field matches the stored password hash for the authenticated user
3. IF the current password validation fails, THEN THE Password_Change_Endpoint SHALL return HTTP 401 with error code `UNAUTHORIZED` and a message indicating the current password is incorrect
4. IF the `new_password` field contains fewer than 8 characters or more than 72 bytes, THEN THE Password_Change_Endpoint SHALL return HTTP 400 with error code `VALIDATION_ERROR` and a message indicating the password length constraint (minimum 8 characters, maximum 72 bytes)
5. IF the request body is missing, not valid JSON, or does not contain both `current_password` and `new_password` string fields, THEN THE Password_Change_Endpoint SHALL return HTTP 400 with error code `VALIDATION_ERROR` and a message indicating the required fields
6. WHEN both validations pass, THE Password_Change_Endpoint SHALL hash the new password with bcrypt (DefaultCost) and update the user record via the existing `UpdateUserPassword` query
7. WHEN the password is updated successfully, THE Password_Change_Endpoint SHALL return HTTP 200 with a JSON body containing a `message` field indicating the password was changed successfully
8. IF the password update fails due to an internal error, THEN THE Password_Change_Endpoint SHALL return HTTP 500 with error code `INTERNAL_ERROR` and a message indicating the password could not be updated

### Requirement 5: Password Change Frontend

**User Story:** As an authenticated user, I want a password change form in my Settings page, so that I can update my password through the UI.

#### Acceptance Criteria

1. THE Settings_UI SHALL display a "Change Password" section with fields for current password, new password, and confirm new password, each accepting between 1 and 128 characters
2. THE Settings_UI SHALL disable the submit button until all three fields contain at least one non-whitespace character and the new password field value matches the confirm new password field value
3. WHEN the new password field contains fewer than 8 characters and the field has been modified, THE Settings_UI SHALL display an inline validation message below the new password field
4. WHEN the form is submitted, THE Settings_UI SHALL disable the submit button and display a loading indicator until the API responds
5. WHEN the API returns a success response, THE Settings_UI SHALL display a success toast notification and clear all three password fields to empty
6. IF the API returns an error, THEN THE Settings_UI SHALL display the error message from the API error envelope as an inline error above the form, preserving the current field values
7. THE Settings_UI SHALL use `t()` function for all user-visible strings in the password change section

### Requirement 6: Documentation Updates

**User Story:** As a self-hosted operator, I want documentation for the admin reset feature and the new password change endpoint, so that I can understand how to use these capabilities.

#### Acceptance Criteria

1. THE `.env.example` file SHALL include the `PULSE_RESET_ADMIN` variable with an inline comment describing its purpose, accepted values (`true`/`false`), default behavior when unset, and a usage example, following the existing comment style in the file
2. THE OpenAPI specification SHALL include the `PUT /api/v1/auth/password` endpoint with an operationId, summary, tags, request body schema (current_password, new_password fields), success response schema (200), and error response entries for 400 (validation failure) and 401 (authentication failure or current password incorrect)
3. THE OpenAPI specification SHALL document `GET /api/v1/auth/setup` with an additional description indicating that when Admin_Reset_Mode is active (`PULSE_RESET_ADMIN=true`), the endpoint returns `setup_required: true` regardless of existing user count
4. THE OpenAPI specification SHALL document that `POST /api/v1/auth/setup`, when Admin_Reset_Mode is active, replaces the existing admin credentials instead of returning 409, and include the corresponding success response schemas (200 for overwrite, 201 for new user) and error responses (400 for validation failure)

### Requirement 7: Locale Completeness for Setup Flow

**User Story:** As a user accessing Pulse in any supported language, I want the setup flow to be fully localized, so that the experience is consistent regardless of language selection.

#### Acceptance Criteria

1. THE setup page SHALL use `t()` function calls for all user-visible strings including titles, labels, placeholders, buttons, and error messages with no hardcoded display text in templates or component scripts
2. THE translation files for all 13 supported locales (en, ar, be, de, es, fr, it, ja, ko, pt, ru, tr, zh) SHALL contain every key referenced by the setup page and the password change section under the `settings.password.*` namespace
3. WHEN new i18n keys are added for the password change feature, THE translation files for all 13 supported locales SHALL be updated with the new keys using English values as placeholders where translations are not yet available
4. WHEN locale changes for the setup flow are complete, THE locale validation script (`frontend/scripts/validate-locales.ts`) SHALL exit with code 0 and report zero errors for all 13 locale files

### Requirement 8: New SQL Query for Admin Reset

**User Story:** As a developer, I want a dedicated SQL query for updating user email and password during admin reset, so that the operation is atomic and maintains data integrity.

#### Acceptance Criteria

1. THE store layer SHALL provide an `UpdateUserEmailAndPassword` query that accepts parameters (id UUID, email TEXT, password_hash TEXT) and updates both the email and password_hash fields in a single UPDATE statement for the specified user ID
2. WHEN the query executes successfully, THE store layer SHALL update the `updated_at` timestamp to the current time and return the updated user record including all columns (via RETURNING *)
3. IF no user exists with the specified ID, THEN THE store layer SHALL return no rows (zero affected rows)
4. IF the provided email conflicts with an existing UNIQUE constraint on the email column, THEN THE store layer SHALL return a unique-violation error to the caller without modifying any data
