# Technical Design: Password Reset

## Overview

This design covers two password management mechanisms for the self-hosted single-user Pulse platform:
1. **Admin Reset Mode** — env-var-driven re-enablement of the setup flow, overwriting existing credentials
2. **Authenticated Password Change** — in-app endpoint + UI for day-to-day password rotation

Both share the principle of keeping credentials in-place (preserving the user UUID and all FK relationships).

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Startup Flow                             │
│                                                                 │
│  config.LoadApp() ──► ResetAdmin: bool ──► log WARNING          │
│                                           (if true)             │
│                                                                 │
│  SetupHandler gets resetAdmin flag via Deps struct              │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    Admin Reset Mode                               │
│                                                                 │
│  GET /api/v1/auth/setup                                         │
│    └─ if resetAdmin → always return setup_required: true        │
│                                                                 │
│  POST /api/v1/auth/setup                                        │
│    └─ if resetAdmin && user exists:                             │
│         → UpdateUserEmailAndPassword (atomic overwrite)         │
│         → Return JWT (HTTP 200)                                 │
│    └─ if resetAdmin && no user:                                 │
│         → CreateUser (same as normal setup)                     │
│         → Return JWT (HTTP 201)                                 │
│                                                                 │
│  Frontend /login checks setup_required → redirects to /setup    │
│  (no frontend changes needed — existing redirect logic works)   │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                Authenticated Password Change                     │
│                                                                 │
│  PUT /api/v1/auth/password  (protected by combinedAuth)         │
│    Request:  { current_password, new_password }                 │
│    Flow:     validate current → validate new length → bcrypt    │
│              → UpdateUserPassword → 200 OK                      │
│                                                                 │
│  Frontend: PasswordChangeSection in /settings page              │
│    └─ 3 fields: current, new, confirm                           │
│    └─ Inline validation + toast on success                      │
└─────────────────────────────────────────────────────────────────┘
```

## Components and Interfaces

### Backend Components

1. **Config Loader** (`internal/config/config.go`)
   - Interface: `LoadApp() → App` struct with new `ResetAdmin bool` field
   - Reads `PULSE_RESET_ADMIN` env var, case-insensitive "true" match

2. **SetupHandler** (`internal/api/handlers/setup.go`)
   - Interface: `NewSetupHandler(queries, jwtSecret, jwtExpiry, resetAdmin) → *SetupHandler`
   - `GET /api/v1/auth/setup` → `Status(c *gin.Context)` — returns `setup_required` based on mode
   - `POST /api/v1/auth/setup` → `Setup(c *gin.Context)` — creates or overwrites user

3. **AuthHandler** (`internal/api/handlers/auth.go`)
   - New method: `ChangePassword(c *gin.Context)`
   - `PUT /api/v1/auth/password` — validates current password, updates to new

4. **Store Layer** (`internal/store/postgres/queries/users.sql`)
   - `UpdateUserEmailAndPassword(ctx, params) → (User, error)` — atomic email+password overwrite
   - `GetFirstUser(ctx) → (User, error)` — retrieves existing user for overwrite

### Frontend Components

5. **PasswordChangeSection** (`routes/settings/PasswordChangeSection.svelte`)
   - Props: none (self-contained, uses API client directly)
   - State: currentPassword, newPassword, confirmPassword, submitting, error
   - Emits: toast on success

6. **API Client** (`lib/api.ts`)
   - New function: `changePassword(data: ChangePasswordRequest) → Promise<{message: string}>`

### Component Interactions

```
[Settings Page] ──imports──► [PasswordChangeSection]
                                    │
                                    ▼
                              [api.changePassword()]
                                    │
                                    ▼
                     PUT /api/v1/auth/password
                                    │
                                    ▼
                   [combinedAuth middleware] ──► [AuthHandler.ChangePassword]
                                                         │
                                                         ▼
                                              [queries.UpdateUserPassword]
```

```
[Login Page] ──onMount──► [api.getSetupStatus()]
                                    │
                                    ▼
                     GET /api/v1/auth/setup
                                    │
                                    ▼
                   [SetupHandler.Status] ──checks──► resetAdmin flag
                                    │                     │
                              (if setup_required)    (if PULSE_RESET_ADMIN=true)
                                    ▼                     ▼
                            redirect to /setup     always returns true
```

## Data Models

### Existing (no schema migration needed)

```sql
-- users table (unchanged)
CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### New SQL Queries (no DDL changes)

```sql
-- Atomic email + password overwrite for admin reset
-- name: UpdateUserEmailAndPassword :one
UPDATE users
SET email = $2, password_hash = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- Get first (only) user for admin reset overwrite target
-- name: GetFirstUser :one
SELECT * FROM users LIMIT 1;
```

### API Request/Response Models

```typescript
// PUT /api/v1/auth/password
interface ChangePasswordRequest {
  current_password: string;  // existing password for verification
  new_password: string;      // new password (8-72 bytes)
}

// 200 OK
interface ChangePasswordResponse {
  message: string;  // "password updated successfully"
}
```

### Config Model

```go
type App struct {
    // ... existing fields ...
    ResetAdmin bool  // PULSE_RESET_ADMIN=true enables admin credential reset
}
```

## Error Handling

| Scenario | HTTP Status | Error Code | Message |
|----------|-------------|------------|---------|
| Missing/invalid JSON body (password change) | 400 | VALIDATION_ERROR | current_password and new_password are required |
| New password < 8 chars | 400 | VALIDATION_ERROR | new password must be at least 8 characters |
| New password > 72 bytes | 400 | VALIDATION_ERROR | new password must be at most 72 bytes |
| Current password incorrect | 401 | UNAUTHORIZED | current password is incorrect |
| DB error on password update | 500 | INTERNAL_ERROR | failed to update password |
| Setup: missing fields (reset mode) | 400 | VALIDATION_ERROR | email and password are required |
| Setup: short password (reset mode) | 400 | VALIDATION_ERROR | password must be at least 8 characters |
| Setup: DB error on overwrite | 500 | INTERNAL_ERROR | failed to update user |
| Setup: user exists, reset mode OFF | 409 | SETUP_ALREADY_COMPLETE | initial setup has already been completed |

## Correctness Properties

### Property 1: FK Integrity
Admin reset NEVER deletes the user row. It updates email/password_hash in-place, preserving the UUID that api_tokens and notification_bindings reference.
**Validates: Requirements 2.1, 2.2**

### Property 2: Atomic Overwrite
`UpdateUserEmailAndPassword` is a single SQL statement — either both fields update or neither does. No partial state.
**Validates: Requirements 8.1, 8.2**

### Property 3: Idempotent Setup Status
Multiple calls to `GET /auth/setup` always return the same result for the same runtime state (no side effects).
**Validates: Requirements 1.3**

### Property 4: Bcrypt Safety
New password validated against 72-byte limit before hashing (bcrypt silently truncates beyond 72 bytes, which could cause unexpected behavior).
**Validates: Requirements 4.4**

### Property 5: Timing Safety
Password verification uses bcrypt.CompareHashAndPassword which is inherently constant-time for the hash comparison.
**Validates: Requirements 4.2, 4.3**

### Property 6: No Credential Leak
Password change response contains only a success message, never echoes the password or hash.
**Validates: Requirements 4.7**

## Detailed Design

### 1. Config Layer

**File:** `backend/internal/config/config.go`

Add `ResetAdmin bool` field to the `App` struct, parsed from `PULSE_RESET_ADMIN` env var with case-insensitive `"true"` matching:

```go
type App struct {
    // ... existing fields ...
    ResetAdmin bool // PULSE_RESET_ADMIN — re-enables setup flow for credential reset
}

func LoadApp() App {
    return App{
        // ... existing ...
        ResetAdmin: strings.EqualFold(getEnv("PULSE_RESET_ADMIN", ""), "true"),
    }
}
```

### 2. Startup Warning

**File:** `backend/cmd/pulse/main.go`

After `config.LoadApp()`, before HTTP server start:

```go
if cfg.ResetAdmin {
    log.Printf("WARNING: Admin reset mode is active. Remove PULSE_RESET_ADMIN after completing setup.")
}
```

### 3. Router Integration

**File:** `backend/internal/api/router.go`

Pass `ResetAdmin` flag through `Deps` struct to the `SetupHandler`:

```go
type Deps struct {
    // ... existing ...
    ResetAdmin bool
}
```

Constructor call:
```go
setupHandler := handlers.NewSetupHandler(deps.Queries, deps.JWTSecret, deps.JWTExpiry, deps.ResetAdmin)
```

Register password change on the protected group:
```go
protected.PUT("/auth/password", authHandler.ChangePassword)
```

The `AuthHandler` already has access to `queries` and `jwtSecret` — add `ChangePassword` method to it.

### 4. Setup Handler Modifications

**File:** `backend/internal/api/handlers/setup.go`

Add `resetAdmin bool` field to `SetupHandler`. Modify behavior:

```go
type SetupHandler struct {
    queries    *db.Queries
    jwtSecret  []byte
    jwtExpiry  time.Duration
    resetAdmin bool  // NEW
}
```

**Status endpoint** — when `resetAdmin` is true, always return `setup_required: true`:

```go
func (h *SetupHandler) Status(c *gin.Context) {
    if h.resetAdmin {
        c.JSON(http.StatusOK, setupStatusResponse{SetupRequired: true})
        return
    }
    // ... existing CountUsers logic ...
}
```

**Setup endpoint** — when `resetAdmin` is true and a user exists, overwrite instead of 409:

```go
func (h *SetupHandler) Setup(c *gin.Context) {
    count, _ := h.queries.CountUsers(ctx)

    if count > 0 && !h.resetAdmin {
        // existing 409 behavior
        apiError(c, http.StatusConflict, "SETUP_ALREADY_COMPLETE", "...")
        return
    }

    // ... validate request (email, password >= 8 chars) ...
    // ... bcrypt hash ...

    if count > 0 && h.resetAdmin {
        // Overwrite existing user
        users, _ := h.queries.ListUsers(ctx) // get first user
        user, err := h.queries.UpdateUserEmailAndPassword(ctx, db.UpdateUserEmailAndPasswordParams{
            ID:           users[0].ID,
            Email:        req.Email,
            PasswordHash: string(hash),
        })
        // ... generate JWT, return 200 ...
    } else {
        // Create new user (existing logic)
        // ... return 201 ...
    }
}
```

**Note:** We need a `ListUsers` query (or `GetFirstUser`) to find the existing user ID when overwriting. Since this is single-user, a simple `SELECT * FROM users LIMIT 1` suffices.

### 5. New SQL Queries

**File:** `backend/internal/store/postgres/queries/users.sql`

```sql
-- name: UpdateUserEmailAndPassword :one
UPDATE users
SET
    email         = $2,
    password_hash = $3,
    updated_at    = now()
WHERE id = $1
RETURNING *;

-- name: GetFirstUser :one
SELECT * FROM users LIMIT 1;
```

After adding, run `sqlc generate` to produce Go code.

### 6. Authenticated Password Change Handler

**File:** `backend/internal/api/handlers/auth.go`

Add `ChangePassword` method to `AuthHandler`:

```go
type changePasswordRequest struct {
    CurrentPassword string `json:"current_password" binding:"required"`
    NewPassword     string `json:"new_password" binding:"required"`
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
    var req changePasswordRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "current_password and new_password are required")
        return
    }

    if len(req.NewPassword) < 8 {
        apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "new password must be at least 8 characters")
        return
    }

    if len(req.NewPassword) > 72 {
        apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "new password must be at most 72 bytes")
        return
    }

    userID := c.GetString("user_id")
    uid, _ := uuid.Parse(userID)

    user, err := h.queries.GetUser(c.Request.Context(), uid)
    if err != nil {
        apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve user")
        return
    }

    if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
        apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "current password is incorrect")
        return
    }

    hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)

    _, err = h.queries.UpdateUserPassword(c.Request.Context(), db.UpdateUserPasswordParams{
        ID:           uid,
        PasswordHash: string(hash),
    })
    if err != nil {
        apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update password")
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "password updated successfully"})
}
```

### 7. Frontend API Client

**File:** `frontend/src/lib/api.ts`

```typescript
export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
}

/** PUT /api/v1/auth/password — change current user password */
export async function changePassword(data: ChangePasswordRequest): Promise<{ message: string }> {
  return apiRequest<{ message: string }>('PUT', '/auth/password', data, { skipToast: true });
}
```

### 8. Frontend Password Change Component

**File:** `frontend/src/routes/settings/PasswordChangeSection.svelte`

A new section component following the existing pattern (`ApiTokenSection.svelte`, `SmtpSettingsSection.svelte`):

- Uses Svelte 5 runes (`$state`, `$derived`)
- Three `<input type="password">` fields
- Inline validation (min 8 chars, passwords match)
- Submits to `changePassword()` API
- Shows toast on success, inline error on failure
- Clears fields on success
- All strings via `t('settings.password.*')`

Integrated into `frontend/src/routes/settings/+page.svelte`:
```svelte
<PasswordChangeSection />
```

### 9. i18n Keys

**Namespace:** `settings.password.*`

New keys added to `en.json` (and all 13 locales as English placeholders):

```json
{
  "settings": {
    "password": {
      "title": "Change Password",
      "description": "Update your account password.",
      "currentPassword": "Current Password",
      "currentPasswordPlaceholder": "Enter current password",
      "newPassword": "New Password",
      "newPasswordPlaceholder": "Enter new password",
      "confirmPassword": "Confirm New Password",
      "confirmPasswordPlaceholder": "Confirm new password",
      "submit": "Change Password",
      "submitting": "Changing…",
      "success": "Password changed successfully.",
      "validation": {
        "minLength": "Password must be at least 8 characters",
        "mismatch": "Passwords do not match"
      },
      "errors": {
        "incorrectCurrent": "Current password is incorrect.",
        "failed": "Failed to change password. Please try again."
      }
    }
  }
}
```

### 10. Documentation Updates

**`.env.example`** — add in Security section:
```bash
# Admin credential reset mode. When set to "true", Pulse re-enables the
# initial setup flow on every startup — allowing you to overwrite the admin
# email and password. The existing user is preserved (API tokens, bindings stay intact).
# Remove this variable after completing the reset.
# PULSE_RESET_ADMIN=true
```

**`backend/api/openapi.yaml`** — add:
- `PUT /api/v1/auth/password` endpoint (request/response/errors)
- Updated descriptions on `GET /api/v1/auth/setup` and `POST /api/v1/auth/setup` noting Admin Reset Mode behavior

**`AGENTS.md`** — add `PULSE_RESET_ADMIN` to the environment variables table.

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `backend/internal/config/config.go` | Modify | Add `ResetAdmin` field |
| `backend/cmd/pulse/main.go` | Modify | Add startup warning, pass `ResetAdmin` to Deps |
| `backend/internal/api/router.go` | Modify | Add `ResetAdmin` to Deps, register `PUT /auth/password` |
| `backend/internal/api/handlers/setup.go` | Modify | Accept `resetAdmin`, overwrite logic |
| `backend/internal/api/handlers/auth.go` | Modify | Add `ChangePassword` handler |
| `backend/internal/store/postgres/queries/users.sql` | Modify | Add 2 new queries |
| `backend/internal/store/postgres/users.sql.go` | Regenerate | `sqlc generate` |
| `frontend/src/lib/api.ts` | Modify | Add `changePassword()` |
| `frontend/src/routes/settings/+page.svelte` | Modify | Import `PasswordChangeSection` |
| `frontend/src/routes/settings/PasswordChangeSection.svelte` | Create | New component |
| `frontend/src/locales/en.json` | Modify | Add `settings.password.*` keys |
| `frontend/src/locales/{ar,be,de,es,fr,it,ja,ko,pt,ru,tr,zh}.json` | Modify | Add same keys as EN placeholders |
| `.env.example` | Modify | Add `PULSE_RESET_ADMIN` |
| `backend/api/openapi.yaml` | Modify | Add `PUT /auth/password`, update setup docs |
| `AGENTS.md` | Modify | Add `PULSE_RESET_ADMIN` to env table |

## Security Considerations

- **Admin Reset Mode** is gated by server-side env var — only operators with SSH/Docker access can trigger it. No API exposure.
- Startup warning ensures operators don't accidentally leave it on.
- **Password Change** requires current password verification (not just auth) — prevents token-stealing attacks from escalating to credential takeover.
- bcrypt 72-byte limit validated server-side (silently truncated by bcrypt otherwise).
- Timing-safe comparison on current password validation (bcrypt is inherently constant-time).

## Testing Strategy

- **Backend:** Unit tests for `ChangePassword` handler (happy path, wrong current pw, short pw, 72-byte limit). Integration test for setup handler in reset mode (overwrite, create-new, validation).
- **Frontend:** Vitest unit test for `PasswordChangeSection` (form validation, submit flow, error display, field clearing).
- **Locale:** Run `pnpm --filter frontend run validate-locales` after adding keys.
