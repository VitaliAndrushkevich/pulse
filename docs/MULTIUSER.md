# Multiuser access

## Scope and decisions

Pulse remains one shared monitoring instance. Users see the same monitoring data;
this change does not add tenants or per-monitor ownership. There is no public
registration, invitation email, or self-service password recovery.

The existing first-run setup creates an administrator. Upgrades retain existing
accounts as administrators without forcing them to change their passwords.
Migration 019 repairs installations that recorded version 018 without its user
columns. It is also safe after a complete version 018 and preserves credentials,
account status, and existing roles.

| Capability | `admin` | `read_only` |
| --- | --- | --- |
| View dashboard, monitors, incidents, history, and heartbeats | Yes | Yes |
| Receive monitoring status updates | Yes | Yes |
| Create, edit, pause, resume, or delete monitoring resources | Yes | No |
| Manage credentials, notification configuration, SMTP, and API tokens | Yes | No |
| Create users, assign roles, disable accounts, reset passwords | Yes | No |
| Change own password and personal display preferences | Yes | Yes |

## Account lifecycle

1. An administrator creates an account with an email, role, and temporary password.
   The administrator delivers those credentials to the user outside Pulse.
2. The user signs in through the existing login page.
3. Pulse requires the user to enter their temporary password and choose a different
   password. Until that succeeds, only their profile and password-change flow are
   available; application APIs and WebSocket connections are blocked.
4. A successful change clears the requirement and issues a new session. The old
   session is invalidated.
5. Administrators can reset another user's password, returning the account to the
   same restricted state. Disabling an account prevents login and further access.

Passwords use the existing bcrypt storage. New passwords must be between 8 and 72
bytes. Passwords and hashes never appear in user-list or profile responses.

## Enforcement

Authorization uses the account's current database state on every authenticated
request, including API-token access and WebSocket handshakes. A JWT's role is not
trusted as the source of permission. Session versions invalidate old JWTs after
password changes, administrative resets, disabling accounts, and role changes.
Existing WebSocket connections recheck credentials every 54 seconds and close
when the check fails. API tokens inherit their owner's current permissions and
are blocked while the account is disabled or awaiting a password change. They
become usable again after re-enabling the account or completing the password
change, unless the administrator explicitly revokes them.

Read-only restrictions are enforced in the API as well as reflected in navigation,
buttons, and direct-route guards. Read access to secrets and administrative settings
is restricted separately from ordinary GET requests. Heartbeat ingestion credentials
must not be disclosed to read-only users. Monitor `settings` is returned as an
empty object for read-only users because arbitrary headers, metadata, and payloads
can contain secrets.

Administrator updates must preserve an active administrator. Account removal is
represented by disabling a user so resource and token ownership remain intact.
The emergency `PULSE_RESET_ADMIN` setup flow must select an administrator
deterministically rather than an arbitrary user.

## Administrator workflow

Open **Settings → Users** to create an account. Select **Read only** or **Admin**
and enter a temporary password. Share the login address, email, and temporary
password with the user. The user must replace that password before proceeding.

The same section lets administrators change another user's role, disable or
re-enable an account, and assign a replacement temporary password. Administrators
cannot disable or demote themselves. Use the personal password section to change
your own password. Disabling keeps historical resource ownership intact.

## Implementation sequence

1. Add the account migration, sqlc queries, user response schema, and API contract.
2. Implement account administration, database-backed authorization, restricted
   first-login sessions, and WebSocket checks.
3. Integrate profile hydration, password-change routing, user administration, and
   permission-aware monitoring pages into the existing UI.
4. Verify the contract and access rules together, including existing-account
   compatibility, API tokens, stale sessions, and direct route navigation.

Backend and frontend implementation are assigned to separate agents. A third agent
reviews access paths and data exposure. Existing uncommitted UI changes are
preserved throughout implementation.

## Acceptance checks

- Existing admin can still sign in and use existing monitoring data.
- Admin can create a user with either role; duplicate emails are rejected.
- New user cannot access monitoring APIs or WebSockets before replacing the
  temporary password, including by calling the endpoints directly.
- Read-only user can inspect monitoring state but cannot mutate it, inspect
  credentials, or manage users or tokens.
- Admin can change roles, reset passwords, and disable accounts without removing
  the final active administrator.
- Password resets and disabled accounts invalidate existing access; old passwords
  and invalidated JWTs cannot regain access.
- Frontend type checks, locales, tests, and build pass; backend race tests and
  build pass; generated sqlc output matches the queries.

The real PostgreSQL upgrade/lifecycle test is opt-in. Create an empty disposable
database whose name starts with `pulse_multiuser_verify_`, then run from `backend/`:

```sh
PULSE_MULTIUSER_TEST_DATABASE_URL='postgres://pulse:pulse@localhost:5432/pulse_multiuser_verify_example?sslmode=disable' \
  go test -tags integration -race -count=1 ./internal/api -run '^TestMultiuserPostgresUpgradeAndLifecycle$'
```

The test refuses databases with another name or an existing `users` table. It
applies migrations through version 17, inserts an existing administrator, upgrades
through version 19, checks recovery from a recorded version 18 with missing user
columns, and exercises the account lifecycle through HTTP. Remove the
disposable database after verification.
