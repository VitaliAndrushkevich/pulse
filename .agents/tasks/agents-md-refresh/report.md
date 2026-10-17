# AGENTS.md refresh — verified findings report

Audit date: working tree at `eef2966` (HEAD) plus uncommitted changes (`git status`: staged `README.md` edit and new `docs/PRODUCT-ROADMAP*.md`, `docs/TECHNICAL-DEBT*.md`; untracked `.kiro/specs/heartbeat-monitoring/`). AGENTS.md was last touched in `f0a2c3e` (2026-08-01).

Every claim below cites the file/line or command checked. "AGENTS says" = current `/AGENTS.md`.

---

## 0. Top corrections (summary)

| # | AGENTS.md claim | Reality (evidence) |
|---|---|---|
| 1 | `make dev` = full stack incl. frontend dev server; `make dev-local` = backend + postgres only | Reversed/wrong. `make dev` = `docker compose up --build` on `docker-compose.yml` → pulls published image `ghcr.io/vitaliandrushkevich/pulse:main` + TimescaleDB (no `build:` key, so `--build` is a no-op, no frontend dev server, postgres port not exposed). `make dev-local` = `docker-compose.dev.yml` → `golang:1.26` backend (`go run`), `node:lts-alpine` Vite frontend on 5173, TimescaleDB on 5432. (Makefile:13-17; docker-compose.yml:3,34-36; docker-compose.dev.yml:7-63) |
| 2 | `make openapi` validates the spec | It only prints `.info.version` via `yq` (or a hint if yq is missing). Ran it: output `1.0.0` / `Done.` (Makefile:61-68). No real validation exists. |
| 3 | `pnpm --filter frontend run validate-locales` | Wrong: no root pnpm workspace and the package is named `pulse-frontend` (frontend/package.json:2). Correct: `cd frontend && pnpm validate-locales` (CI does exactly this: pull_request_opened.yml "Validate locales"). Also auto-run by `pnpm build` (package.json:9). |
| 4 | `PULSE_NOTIFICATION_WORKERS` default `50` | Code default is **10** (backend/cmd/pulse/main.go:125). `.env.example` sets 50 explicitly. |
| 5 | `DATABASE_URL` *(required)* | Has a default `postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable` (backend/internal/config/config.go:11,42). |
| 6 | Env tables (two, partial) | 8 backend vars missing (see §1): `PULSE_JWT_EXPIRY`, `PULSE_OPENAPI_DIR`, `PULSE_RETENTION_CHECK_INTERVAL`, `PULSE_MAX_METRIC_TAG_KEYS`, `PULSE_SECRET_KEY_NEW`, `MIGRATIONS_PATH`, `PULSE_RUN_DB_TESTS`, `VITE_API_PROXY_TARGET`, plus all `PULSE_MCP_*`. |
| 7 | Delivery Constraints: "alert channels" future scope | Notifications (email + webhook) are implemented (migrations 014/015, `backend/internal/notification/`, router.go:139-147). Remove "alert channels". |
| 8 | WS message types: `connected`, `monitor_status` | Third type exists: `monitor_tags_changed` (backend/internal/hub/messages.go:8-12,87). |
| 9 | "history (TimescaleDB 7-day window)" | Per-monitor `history_retention_days` 1–365, default 30 (migration 011); retention service (`backend/internal/retention/`); history query `from`/`to`/`step`, default last 24h (handlers/history.go:75,166-197). |
| 10 | "Secrets … management", "Settings (secrets)" | Legacy `/api/v1/secrets` removed from router (router.go:110-112); replaced by per-monitor credentials `/monitors/{id}/credentials`. Settings page sections: tokens, password, language (routes/settings/+page.svelte). |
| 11 | Reminders "re-enqueue … while condition persists" | `ActivateReminder` has **no production caller** (`grep -rn ActivateReminder backend` → only definition at notification/reminder.go:73). Confirms docs/TECHNICAL-DEBT.md TD-004. Don't claim reminders work end-to-end. |
| 12 | `make rotate-key` "AES key rotation" | Rotates legacy `secrets` + monitor credentials only (cmd/rotate/main.go:69,110,138); SMTP password and webhook headers NOT rotated (TD-008). |
| 13 | Link `docs/MILESTONES.md` | File does not exist; deleted in `da598da` ("chore(docs): restructure documentation"). |
| 14 | Key Files lists 3 workflows | 6 exist (see §6). |
| 15 | "218 frontend tests passing" | Volatile; 48 `*.test.ts` files now (`find frontend/src -name '*.test.ts' \| wc -l`). Remove the count. |
| 16 | Heartbeat monitoring | Not mentioned; it is **planned only** (§7). Must not be listed as done. |

---

## 1. Verified complete environment variable table

Method: `grep` for `PULSE_*`, `DATABASE_URL`, `VITE_*`, `os.Getenv`, `import.meta.env`, `process.env` across `backend/`, `mcp/`, `frontend/src`, `frontend/vite.config.ts`, excluding tests (plus test gates listed separately).

### Pulse server (`backend/cmd/pulse`)

| Variable | Default (code) | Notes | Source |
|---|---|---|---|
| `PULSE_PORT` | `8080` | | config.go:10,41 |
| `PULSE_DEV` | disabled | Only the exact string `true` enables. Enables Swagger UI (`/swagger`, `/swagger/openapi.yaml`) and allows all WS origins. | config.go:46; router.go:58-61; handlers/swagger.go:22-27; handlers/ws.go:48-49 |
| `PULSE_SECRET_KEY` | **required** | Base64 (std) of exactly 32 bytes, else fatal. | crypto/crypto.go:17-33; main.go:43-46 |
| `PULSE_JWT_SECRET` | **required** | Non-empty, else fatal. | config.go:44; main.go:49-51 |
| `PULSE_JWT_EXPIRY` | `24h` | Go duration; invalid → fatal. | config.go:14,45; main.go:52-55 |
| `DATABASE_URL` | `postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable` | Also read by `cmd/migrate` and `cmd/rotate`. | config.go:11,42,58; cmd/rotate/main.go:45-48 |
| `PULSE_SCHEDULER_WORKERS` | `50` | Positive int. `.env.example` recommends 200 for prod. `monitor.DefaultWorkers=200` (scheduler.go:25) is unreachable from main because config always passes >0. | config.go:13,43; scheduler.go:24-27,57-58 |
| `PULSE_BASE_URL` | empty | Trailing `/` trimmed. Used for email links and WS Origin check (scheme+host); empty → all origins allowed. | config.go:48; ws.go:48-66 |
| `PULSE_METRICS_USER` / `PULSE_METRICS_PASSWORD` | empty | Basic Auth on `/metrics` only when **both** set. | config.go:49-50; main.go:197-201; handlers/metrics.go:59-61 |
| `PULSE_RESET_ADMIN` | disabled | Case-insensitive `true` re-enables setup flow; logs a warning. | config.go:51; main.go:38-40 |
| `PULSE_OPENAPI_DIR` | `api` (relative to CWD) | Swagger source in dev. Dev compose sets `/workspace/backend/api`; Dockerfile copies `api/` to `/app/api`. | config.go:47; docker-compose.dev.yml:18; Dockerfile:40 |
| `PULSE_RETENTION_CHECK_INTERVAL` | `1h` | Range `[1m, 168h]`; out of range/invalid → fatal. | retention/config.go:8-42; main.go:102-105 |
| `PULSE_NOTIFICATION_WORKERS` | **`10`** | Dispatcher buffer fixed at 256. | main.go:125-130 |
| `PULSE_NOTIFICATION_DRAIN_TIMEOUT` | `30s` | | main.go:126; dispatcher.go:101 |
| `PULSE_LOG_LEVEL` | `warn` | `info`/`debug` → info level (notification delivery logs). | notification/dispatcher.go:31-34 |
| `PULSE_MAX_METRIC_TAG_KEYS` | `10` | Max tag keys exposed as Prometheus labels. | monitor/dynamic_metrics.go:15,74-77 |

### Other binaries / tooling

| Variable | Default | Used by | Source |
|---|---|---|---|
| `PULSE_SECRET_KEY_NEW` | required for rotation | `make rotate-key` (`cmd/rotate`) | cmd/rotate/main.go:34 |
| `MIGRATIONS_PATH` | `file://migrations` | `cmd/migrate` | config.go:12,60 |
| `PULSE_RUN_DB_TESTS` | unset (skip) | Set `1` to run TimescaleDB integration tests | internal/store/timescale/store_test.go:20-21 |
| `VITE_API_PROXY_TARGET` | `http://localhost:8080` | Vite dev proxy for `/api` (+ ws) | frontend/vite.config.ts:8-10; docker-compose.dev.yml:39 |
| `POSTGRES_PASSWORD` | — | Only in `.env.example`; **not read by Pulse** and not referenced by `docker-compose.yml` (hardcodes `pulse`) | .env.example; docker-compose.yml:32 |

### MCP server (`mcp/`)

| Variable | Default | Source |
|---|---|---|
| `PULSE_MCP_API_TOKEN` | **required** (empty/whitespace → abort) | mcp/internal/config/config.go:57-60 |
| `PULSE_MCP_API_BASE_URL` | `http://localhost:8080/api/v1` | config.go:44,63 |
| `PULSE_MCP_ACCESS_MODE` | `read-only` (anything but `read-write`) | config.go:45,65,76-81 |
| `PULSE_MCP_TRANSPORT` | `stdio` (or `http`) | config.go:46,66 |
| `PULSE_MCP_HTTP_ADDR` | `:9090` | config.go:47,67 (missing from `.env.example`) |
| `PULSE_MCP_REQUEST_TIMEOUT` | `15s` | config.go:48,68,85-94 |

### Planned, NOT implemented
- `PULSE_HEARTBEAT_WORKERS` (default 50) — only in `.kiro/specs/heartbeat-monitoring/design.md:561`. Do not document as live.

---

## 2. Makefile targets and pnpm scripts

### Root `Makefile` (all 13 targets that exist)
`dev`, `dev-local`, `dev-down`, `dev-local-down`, `run`, `build`, `build-frontend`, `build-all`, `test`, `migrate`, `migrate-down`, `rotate-key`, `openapi`.

| Target | Actual behavior |
|---|---|
| `dev` | `docker compose up --build` on `docker-compose.yml` → published `:main` image + TimescaleDB (prod-like) |
| `dev-local` | `docker compose -f docker-compose.dev.yml up --build` → backend (`go run`) + Vite frontend (5173) + TimescaleDB (5432) |
| `dev-down` / `dev-local-down` | `down -v` — **deletes volumes** (DB data). Worth a warning. |
| `run` | `cd backend && go run ./cmd/pulse` with dev env (PULSE_DEV=true, dev secret key, workers 50) — needs postgres on `localhost:5432` |
| `build` | `cd backend && go build -ldflags "-X …/version.Version=$(VERSION)" ./cmd/pulse` |
| `build-frontend` | `pnpm run build` then copy `frontend/build/*` → `backend/internal/frontend/dist/` |
| `build-all` | `build-frontend` + `go build -o pulse` (binary with embedded UI) |
| `test` | `cd backend && go test ./...` (no `-race`; CI uses `-race -count=1`) |
| `migrate` / `migrate-down` | `go run ./cmd/migrate -direction up|down` (flags also `-steps`, cmd/migrate/main.go:18-19) |
| `rotate-key` | `go run ./cmd/rotate` (needs `PULSE_SECRET_KEY` + `PULSE_SECRET_KEY_NEW`) |
| `openapi` | prints spec version via `yq`; **not a validator** |

Notes: `.PHONY` omits `dev-local` and `dev-local-down` (Makefile:1). Migrations are also **auto-applied at server startup** from embedded files (main.go:71-89, `backend/migrations/migrations.go`), so `make migrate` is optional for normal dev.

### `mcp/Makefile`
`build` (→ `mcp/bin/pulse-mcp`), `test` (`go test ./...`), `lint` (`golangci-lint run ./...`; not installed locally — `which golangci-lint` → not found; no `.golangci.*` config in repo), `clean`.

### `frontend/package.json` scripts (all 6)
`dev`, `build` (`svelte-kit sync && pnpm validate-locales && vite build`), `validate-locales` (`tsx scripts/validate-locales.ts`), `preview`, `check` (`svelte-kit sync && svelte-check`), `test` (`vitest --run`).
No `lint`/`format` scripts exist. `packageManager: pnpm@10.34.5`.

Local note: `frontend/node_modules` is empty in this checkout; `pnpm validate-locales` failed with `sh: tsx: command not found`. `pnpm install` is a prerequisite and AGENTS should say so.

### Named in AGENTS but wrong/missing
- `pnpm --filter frontend run validate-locales` → wrong (see §0 #3).
- `make openapi` → mis-described.
- `make dev` / `make dev-local` → mis-described.

### Useful commands to add (all verified to exist)
- Frontend (from `frontend/`): `pnpm install`, `pnpm check`, `pnpm validate-locales`, `pnpm test`, `pnpm build`.
- Backend CI-equivalent: `cd backend && go test -race -count=1 ./...`.
- DB integration tests: `cd backend && PULSE_RUN_DB_TESTS=1 DATABASE_URL=postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable go test ./internal/store/timescale/...` (needs `make dev-local` postgres on 5432).
- gRPC integration tests: `cd backend && go test -tags integration ./internal/monitor/...` (`grpc_integration_test.go:1` has `//go:build integration`).
- sqlc: `cd backend && sqlc generate` (config `backend/sqlc.yaml`: queries `internal/store/postgres/queries`, schema `migrations`, out `internal/store/postgres`). `sqlc diff` ran clean (exit 0) — generated code is in sync.
- MCP: `cd mcp && make test` / `make build` (CI: `go test -race -count=1 ./...`, `CGO_ENABLED=0 go build ./main.go`).
- Embedded build: `make build-all`.

---

## 3. Endpoint surface and MCP co-change list

### Non-`/api/v1` routes (router.go)
`GET /healthz`, `GET /metrics` (optional Basic Auth), `GET /ws` (when hub present), dev-only `GET /swagger`, `GET /swagger/openapi.yaml`, SPA `NoRoute` fallback (JSON 404 for `/api/`, `/ws`, `/metrics`, `/healthz`, `/swagger` prefixes, router.go:285).

### `/api/v1` (from handler `Register` calls; ✓ = in `backend/api/openapi.yaml`)
- Public: `GET /healthz` (✗ not in spec), `POST /auth/login` ✓, `GET|POST /auth/setup` ✓
- Protected (combined JWT/API-token auth, router.go:95-96):
  - `PUT /auth/password` ✓
  - `GET|POST /tokens`, `DELETE /tokens/{id}` (`?permanent=true` hard-deletes, tokens.go:205-223) ✓
  - `GET|POST /monitors`, `GET|PUT|DELETE /monitors/{id}` ✓
  - `GET /monitors/{id}/history` ✓, `GET /monitors/{id}/stats` ✓, `GET /monitors/{id}/incidents` ✓, `GET /incidents` ✓
  - `GET|POST /monitors/{id}/credentials` ✓, `PUT|DELETE /monitors/{id}/credentials/{credentialId}` ✓, **`GET /monitors/{id}/credentials/{credentialId}` ✗ missing from spec** (credentials.go:323)
  - `POST|GET|DELETE /monitors/{id}/proto-source`, `POST /monitors/{id}/proto-source/reflect` ✓; `POST /grpc/reflect`, `POST /grpc/parse-proto` ✓
  - `GET /tags`, `GET /tags/{key}` ✓
  - `GET /dashboard/summary` ✓
  - Notifications: channels CRUD + `/test` + `/delivery-logs`, `template-variables`, `smtp-settings` GET/PUT/DELETE + `/test` ✓
  - Bindings: `POST|GET /monitors/{id}/notification-bindings`, `PUT|DELETE …/{bindingId}` ✓; **`GET /monitors/{id}/delivery-logs` ✗ missing from spec and from AGENTS' notification table** (notification_bindings.go:34)
- Dead code: `handlers/secrets.go` still defines `/secrets` routes but `NewSecretHandler` is never called outside tests; router comment says the table "will be dropped in migration 007", but migration 007 is `check_results_ssl` and no migration drops `secrets` (`grep "DROP TABLE" backend/migrations/*.up.sql` → none for secrets).

The OpenAPI path count is 46 operations (scripted extraction of `paths:`).

### MCP co-change list — verified
`mcp/internal/pulseapi/http_client.go` calls exactly: `GET /monitors` (:59), `GET /monitors/{id}` (:81, :185), `GET /monitors/{id}/stats` (:91), `GET /monitors/{id}/history` (:109), `GET /monitors/{id}/incidents` / `GET /incidents` (:122-137), `POST /monitors` (:167), `DELETE /monitors/{id}` (:176), `PUT /monitors/{id}` (:201, pause/resume via GET+PUT), `GET /dashboard/summary` (:211), `GET /tags` (:221), `GET /tags/{key}` (:231).
**The current AGENTS list of 12 endpoints is exactly correct.** Keep it.

MCP tools (mcp/internal/server/register.go:17-72): read — `list-monitors`, `get-monitor`, `monitor-stats`, `monitor-history`, `downtime-summary`, `list-incidents`, `dashboard-summary`, `list-tags`; write (only when `read-write`, register.go:94-95) — `create-monitor`, `delete-monitor`, `pause-monitor`, `resume-monitor`. Entrypoint is `mcp/main.go` (no `mcp/cmd/`).

---

## 4. Verified versions

| Component | Version | Source |
|---|---|---|
| Go (backend) | `go 1.26.0` | backend/go.mod |
| Go (mcp) | `go 1.25.0` | mcp/go.mod |
| gin | v1.12.0 | backend/go.mod |
| pgx | v5.10.0 | backend/go.mod |
| gorilla/websocket | v1.5.3 | backend/go.mod |
| golang-jwt/jwt | v5.3.1 | backend/go.mod |
| golang-migrate | v4.19.1 | backend/go.mod |
| prometheus/client_golang | v1.24.1 | backend/go.mod |
| grpc / quic-go / miekg/dns | v1.83.2 / v0.62.0 / v1.1.73 | backend/go.mod |
| pgregory.net/rapid (PBT) | v1.3.0 | both go.mod |
| MCP go-sdk | v1.7.0 | mcp/go.mod |
| Svelte | ^5.57.0 | frontend/package.json |
| SvelteKit | ^2.70.3 | frontend/package.json |
| Tailwind CSS | ^3.4.19 | frontend/package.json |
| Vite / Vitest / TypeScript | ^6.4.3 / ^3.2.7 / ^5.9.3 | frontend/package.json |
| fast-check / @testing-library/svelte | ^4.9.0 / ^5.4.2 | frontend/package.json |
| pnpm | 10.34.5 (`packageManager`) | frontend/package.json:6 |
| TimescaleDB image | `timescale/timescaledb:2.28.3-pg17` (both compose files); **CI uses `timescale/timescaledb:latest-pg16`** | docker-compose*.yml; pull_request_opened.yml |
| Docker stages | `node:lts-alpine` → `golang:1.26-alpine` → `gcr.io/distroless/static-debian12` | Dockerfile |

Other constants verified: hub broadcast buffer 256 and per-client send buffer 256 (hub.go:25,56,140); notification dispatcher buffer 256 (main.go:130); API client timeout 15s (lib/api.ts:68); WS backoff 1s–30s ±25% (lib/ws.ts:70-80); VirtualList ≤60 rows (VirtualList.svelte:6,59); retry 30s initial, max 4 attempts (retry.go:15,23-24); reminder interval 30–1440 min (notification_bindings.go:41,105); threshold 1–60000 ms, days_before 1–365 (validation.go:64-80); delivery log error detail 1024 chars (delivery_log.go:13).

Recommendation: keep only stable majors in AGENTS (Go 1.26, gin v1.12, pgx v5, jwt v5, Svelte 5, SvelteKit 2, Tailwind 3.4, TS strict) and point to `go.mod`/`package.json` for exact versions. Drop exact patch versions and the TimescaleDB tag (or say "see compose files").

---

## 5. Locales

- 13 locales registered in `frontend/src/lib/i18n/config.ts:8-20` (en, ar[rtl], be, de, es, fr, it, ja, ko, pt, ru, tr, zh) and 13 files in `frontend/src/locales/`. AGENTS' 13-row table is correct.
- All 12 non-English files have the full key set: 629 keys in `en.json`, 0 missing / 0 extra per locale (scripted flatten+diff).
- Invocation: `cd frontend && pnpm validate-locales` (also runs inside `pnpm build`; CI runs it as its own step).
- **Gap:** `frontend/scripts/validate-locales.ts` hardcodes its own 11-locale list (no `ar`, no `be`) instead of importing `config.ts`, and missing keys only **warn** (exit 0); it fails only on missing files/nesting >4 levels (script header lines 1-11). So the AGENTS checklist step "run validate-locales to detect missing keys" overstates what it guarantees. AGENTS should say: keep `scripts/validate-locales.ts` locale list in sync when adding a language.
- `frontend/scripts/generate-locales.mjs` also exists (bulk translation seeding helper) — undocumented.
- Recommendation: drop the hard number from the heading ("Supported Locales") or keep the table but point to `config.ts` as the source of truth.

---

## 6. GitHub workflows

| File | Trigger | What it does |
|---|---|---|
| `pull_request_opened.yml` ("PR Checks") | `pull_request` → `main`; concurrency per PR | **backend**: TimescaleDB `latest-pg16` service, `go mod download`, `go test -race -count=1 ./...`, `CGO_ENABLED=0 go build ./cmd/pulse`. **mcp**: `dorny/paths-filter` on `mcp/**`, then `go test -race -count=1 ./...` + build. **frontend**: `pnpm install --frozen-lockfile`, `pnpm check`, `pnpm validate-locales`, `pnpm test`, `pnpm build`. |
| `main-push.yml` ("Main Push") | push → `main` | `verify-mcp` (build + `go test -race`), `docker-latest` (multi-arch amd64/arm64 push `:latest`) |
| `pull_request_merged.yml` ("Build & Push (main)") | push → `main` (despite the filename) | Single-arch image push `:main` + `:<sha>` |
| `release-drafter.yml` | push → `main` | Updates draft release (`.github/release-drafter.yml` config) |
| `release.yml` | tag push `v*` | Cross-compile `pulse` + `pulse-mcp` (linux/darwin × amd64/arm64) with embedded frontend; multi-arch Docker push with semver tags; attach binaries + `checksums-sha256.txt` to the release |
| `release-published.yml` ("Release Image") | `release: published` | Single-arch image push with semver tags **and `:latest`** |

PR jobs to mirror in AGENTS "before you push": backend `go test -race -count=1 ./...` + build; frontend `pnpm check && pnpm validate-locales && pnpm test && pnpm build`; mcp `go test -race -count=1 ./... && go build ./main.go` when `mcp/**` changes.

Observations (not AGENTS fixes, but relevant):
- The CI TimescaleDB service is effectively unused: the only DB test requires `PULSE_RUN_DB_TESTS=1`, which CI does not set; `integration`-tagged tests are not run.
- Every push to `main` runs two Docker builds (`main-push` → `:latest` multi-arch; `pull_request_merged` → `:main`/`:sha` single-arch). `:latest` is therefore pushed both on every main push and on release publish, so `:latest` tracks main tip, not the last release. Tag push (`release.yml`, multi-arch) and release publish (`release-published.yml`, single-arch) both push the same semver tags — the later one can overwrite a multi-arch manifest with amd64-only.
- `release.yml` uses older action majors (checkout@v4, setup-go@v5, buildx@v3) vs v7/v4 elsewhere.
- `docker-compose.yml` uses `:main`, i.e. the `pull_request_merged.yml` tag.

---

## 7. Heartbeat monitoring status: PLANNED (spec complete, implementation not started)

Evidence (unambiguous):
- `.kiro/specs/heartbeat-monitoring/` is **untracked** in git (`git status`: `?? .kiro/specs/heartbeat-monitoring/`), files dated Oct 16.
- `tasks.md`: **0 of 36 tasks checked** (`grep -c '- \[x\]'` → 0, `'- \[ \]'` → 36).
- `design-review.json`: `"verdict": "APPROVED"` with NIT-level findings only — design is approved, not built.
- No code: `grep -rli heartbeat backend mcp` → no hits; no heartbeat paths in `openapi.yaml`; no migration 017 (latest is `016_quic_monitor_type`). Frontend hits (`StatusTimeline.svelte:242` "Heartbeat bar", a test comment) are the existing check-history bar UI, unrelated.
- `docs/PRODUCT-ROADMAP.md:42` lists PF-01 as "Next".
- The spec intends to add `PULSE_HEARTBEAT_WORKERS` and update AGENTS.md "as part of the change" (design.md:561).

Recommendation: AGENTS.md may list it under a short "Planned / in-flight specs" line pointing to `.kiro/specs/heartbeat-monitoring/`, clearly marked not implemented. Do not add endpoints, env vars, or tables for it.

---

## 8. Feature inventory (to replace the per-milestone changelog)

Replace "Current Progress" + "Completed (A–H/I/J/CI)" with a compact inventory, pointing to `.kiro/specs/<name>/` for detail. Status from `tasks.md` checkbox counts:

| Area | What exists (evidence) | Spec |
|---|---|---|
| Monitor engine | 10 types: http, http3, tcp, udp, websocket, grpc, dns, icmp, smtp, quic (migration 016 CHECK; `backend/internal/monitor/*.go`); bounded scheduler; LISTEN/NOTIFY wakeups (`monitor/notify.go:23`) | grpc-monitor (28/28) |
| gRPC proto payloads | proto upload/reflection, ad-hoc reflect/parse (`internal/proto/`, `handlers/proto_source.go`, migration 013) | grpc-proto-payload (29/29) |
| Monitor CRUD | idempotent PUT, filtering/ordering/pagination (property tests in handlers) | — |
| Tags | `/tags`, `/tags/{key}`, `monitor_tags_changed` WS message, tag-labelled Prometheus metrics capped by `PULSE_MAX_METRIC_TAG_KEYS` (migration 009, `internal/tags/`, `monitor/dynamic_metrics.go`) | monitor-tagging (45/45) |
| Credentials | per-monitor encrypted credentials replacing legacy secrets API (migration 006, `handlers/credentials.go`, `credential_crypto.go`) | secrets-and-auth-rework (52/52) |
| History & retention | history explorer (from/to/step), per-monitor retention 1–365d, retention service (migration 011, `internal/retention/`) | monitor-history-explorer (29/29) |
| Stats | `/monitors/{id}/stats` uptime % + SSL info (migration 007 SSL columns) | — |
| Dashboard | `/dashboard/summary`; widgets in `frontend/src/components/dashboard/` | dashboard-refactor (37/37) |
| Status timeline | failure details in `StatusTimeline.svelte` | status-timeline-failure-details (18/18) |
| Notifications | email/webhook channels, bindings, triggers, retry, delivery logs (migrations 014/015). Known gaps: TD-003/004/005/008 in `docs/TECHNICAL-DEBT.md` | notification-channels (49 done, 17 optional open) |
| Auth | JWT + API tokens (bcrypt, prefix lookup, permanent delete), first-run setup, password change, `PULSE_RESET_ADMIN` | password-reset (33/33) |
| Realtime | WS hub, diff/patch messages | — |
| Frontend | SvelteKit 5 static SPA; routes: `/`, `/login`, `/setup`, `/monitors`, `/monitors/create`, `/monitors/[id]`, `/monitors/[id]/edit`, `/notifications`, `/notifications/create`, `/notifications/[id]/edit`, `/settings` | frontend-product (46/46) |
| i18n | 13 locales, RTL | i18n-localization (29/29) |
| Branding/theming | lockup, theme switcher, tokens | pulse-branding (28/28) |
| MCP server | 12 tools, stdio/http transports | mcp-server (50/50) |
| Packaging/CI | Dockerfile, compose, 6 workflows | packaging-release (12/12), pnpm-frontend-dev (20/20) |
| Audit | `docs/AUDIT-Jul-2026.md` | pulse-audit-report (28/28) |
| **Heartbeat monitoring** | **not implemented** | heartbeat-monitoring (0/36) |

Docs to point to: `docs/DEVELOPMENT.md`, `docs/API.md`, `docs/AUDIT-Jul-2026.md`, `docs/PRODUCT-ROADMAP.md`, `docs/TECHNICAL-DEBT.md` (the last two are staged but not yet committed), `.kiro/specs/`.

---

## 9. Paths

### Backticked/linked paths in AGENTS.md that do not exist
Scripted check of every backticked token containing `/` (brace globs expanded):
- `docs/MILESTONES.md` — **missing** (deleted in `da598da`). Referenced in "Current Progress".
- All other file/dir paths exist. (False positives skipped: package names like `pgx/v5`, `gorilla/websocket`, `text/template`, `select/default`, and `pulse:pulse@localhost:5432/pulse`.)
- Minor: `backend/internal/api/handlers/smtp_settings.go` exists but its methods hang off `NotificationChannelHandler`; routes are registered in `notification_channels.go:88-91`.

### Important missing paths to add to Key Files
- Config: `backend/internal/config/config.go`
- Auth/middleware: `backend/internal/api/middleware/{auth.go,jwt.go,logging.go}`; combined auth lives in `router.go` (`combinedAuth`)
- Handlers: `backend/internal/api/handlers/{monitors,credentials,credential_crypto,tags,monitor_stats,dashboard,history,incidents,proto_source,setup,auth,tokens,helpers}.go`
- Crypto/tokens: `backend/internal/crypto/crypto.go`, `backend/internal/token/token.go`
- Other packages: `backend/internal/proto/`, `backend/internal/retention/`, `backend/internal/tags/`, `backend/internal/monitor/dynamic_metrics.go`, `backend/internal/monitor/notify.go` (LISTEN/NOTIFY listener), `backend/internal/frontend/frontend.go` (go:embed), `backend/internal/version/`
- Commands: `backend/cmd/migrate/main.go`, `backend/cmd/rotate/main.go`
- sqlc: `backend/sqlc.yaml`, `backend/internal/store/postgres/queries/`
- MCP: `mcp/main.go`, `mcp/internal/config/config.go`, `mcp/internal/server/register.go`, `mcp/internal/tools/`, `mcp/internal/pulseapi/http_client.go`, `mcp/Makefile`, `mcp/README.md`
- Frontend: `frontend/vite.config.ts`, `frontend/src/components/dashboard/`, `frontend/src/lib/stores/patchBus.svelte.ts`, `frontend/scripts/generate-locales.mjs`
- Infra/docs/tooling: all 6 workflows, `docker-compose.yml`, `docker-compose.dev.yml`, `Dockerfile`, `.env.example`, `docs/`, `.kiro/specs/`, `.kiro/hooks/`, `.atl/skill-registry.md`, `skills-lock.json`

### Agent tooling facts worth documenting
- `.kiro/hooks/` (4, all enabled): `locale-sync-check` (on `frontend/**` edit → enforce `t()` + keys in all locales), `openapi-mcp-sync` (on `openapi.yaml` edit → check MCP tools), `update-docs-on-change` (on source/config edit → update README/docs), `update-swagger-documentation` (on `backend/**` edit → sync OpenAPI).
- No `.kiro/steering/` directory exists.
- Skills: `skills-lock.json` pins 23 skills (21 `golang-*` from `samber/cc-skills-golang`, `svelte-code-writer` + `svelte-core-bestpractices` from `sveltejs/ai-tools`). `.atl/skill-registry.md` indexes them at `.agents/skills/<name>/SKILL.md`, but `.agents/skills` is gitignored (`.gitignore`) and **not present** in this checkout (`.agents/` contains only `tasks/`). AGENTS should name the registry and say skills must be restored from `skills-lock.json`.
- `.github/copilot-instructions.md`, `.github/instructions/go.instructions.md`, `.github/prompts/plan-pulseUptimeMonitor.prompt.md` exist (Copilot-side guidance).

---

## 10. Staleness in OTHER docs (list only — not edited)

- **`.github/copilot-instructions.md`**: says "~25% of MVP", milestones C–H "Todo"; "Go 1.22+"; lists `secrets` as a current table; links deleted `docs/MILESTONES.md`.
- **`README.md`**: `PULSE_SCHEDULER_WORKERS` default `200` (code: 50); `PULSE_NOTIFICATION_WORKERS` default `50` (code: 10); env table omits `PULSE_RESET_ADMIN`, `PULSE_RETENTION_CHECK_INTERVAL`, `PULSE_MAX_METRIC_TAG_KEYS`, `PULSE_OPENAPI_DIR`; "Data Flow" checker list omits QUIC; has an uncommitted staged edit.
- **`docs/DEVELOPMENT.md`**: `make dev`/`make dev-local` descriptions wrong (same as AGENTS); omits `dev-down`/`dev-local-down`; "Local Setup" runs `docker compose up postgres -d` using `docker-compose.yml`, whose postgres port is commented out, so `make run` cannot reach `localhost:5432` (use `docker compose -f docker-compose.dev.yml up postgres -d`); CI section omits the MCP job; prerequisites "PostgreSQL 16 / TimescaleDB 2.17+ / pnpm 9+" vs compose pg17/2.28.3 and pnpm 10.34.5.
- **`docs/API.md`** lines 89-90: still lists `POST/GET /api/v1/secrets` (removed from router).
- **`docs/MILESTONES.md`**: does not exist; still linked from AGENTS.md and copilot-instructions.md.
- **`.env.example`**: "Metrics" section duplicated; `POSTGRES_PASSWORD` comment claims compose refuses to start without it, but `docker-compose.yml` doesn't reference it; MCP access-mode comment says read-write exposes only `create-monitor` (actually also delete/pause/resume); omits `PULSE_MCP_HTTP_ADDR`, `PULSE_RETENTION_CHECK_INTERVAL`, `PULSE_MAX_METRIC_TAG_KEYS`, `PULSE_OPENAPI_DIR`.
- **`docker-compose.yml`**: placeholder `PULSE_SECRET_KEY: "pulse-secret-key-change-in-production"` is not valid base64 (python `b64decode(validate=True)` → "Non-base64 digit found"), so the container exits at startup until replaced (arguably intentional fail-fast, but undocumented).
- **`backend/api/openapi.yaml`**: missing `GET /monitors/{id}/credentials/{credentialId}`, `GET /monitors/{id}/delivery-logs`, `GET /api/v1/healthz`.
- **`backend/internal/api/router.go:110-112`** comment: says secrets table dropped in migration 007 — not true.
- **`frontend/scripts/validate-locales.ts`**: locale list missing `ar`, `be`.
- **`.atl/skill-registry.md`**: points to `.agents/skills/*` which is not present locally.
- Module path inconsistency: backend `github.com/VitaliAndrushkevich/pulse` vs mcp `github.com/vandrushkevich/pulse/mcp` (cosmetic).

---

## 11. Open questions for the user

1. **Heartbeat**: OK to add a one-line "Planned (not implemented): heartbeat monitoring — `.kiro/specs/heartbeat-monitoring/`" to AGENTS.md, or omit it entirely until implementation starts?
2. **Section depth**: the Notifications and i18n deep-dives make AGENTS.md long. Keep them inline (trimmed and corrected), or move detail to `docs/` and keep only rules + links in AGENTS.md? (Recommendation: keep rules/checklists inline, move descriptive detail out.)
3. **Uncommitted docs**: `docs/PRODUCT-ROADMAP.md` and `docs/TECHNICAL-DEBT.md` are staged but not committed. OK to link them from AGENTS.md?
4. **Future-scope list**: drop "alert channels" (implemented). Keep "multi-tenant, status page, Terraform provider" as "do not implement unless requested", given the roadmap lists PF-05 status pages and a later Terraform option? (None exist in code — no matches for status page/tenant/terraform under `backend/internal`, `backend/migrations`, `frontend/src`.)
5. **Out-of-scope fixes**: should the bugs found outside AGENTS.md (README/DEVELOPMENT/API.md/copilot-instructions staleness, OpenAPI drift for 2 routes, `validate-locales.ts` missing ar/be, CI `:latest`/semver tag overlap, unused CI DB service) be handled in a follow-up task? This step did not touch them.
6. **`make openapi`**: describe it honestly as "print spec version", or do you want a real validator added later (separate change)?

---

## 12. Changes applied (iteration 1)

Only `AGENTS.md` was edited. No git write commands were run. Size: ~28.8 KB (was ~22 KB), but the added bytes are workflow recipes, a complete env table, and an accurate Key Files map; the per-milestone changelog (~6 KB) is gone.

### What changed in AGENTS.md and why
| Section | Change | Reason (report §) |
|---|---|---|
| New "Repository Map" | backend/frontend/mcp/docs/.kiro/specs at a glance | Fast orientation |
| Architecture Rules | Added embed location + "migrations auto-applied on startup" | §2 |
| API Conventions | Listed root-level exceptions (`/healthz`, `/metrics`, `/ws`, `/swagger`); added "known spec drift" line for 3 routes | §3 |
| WebSocket Protocol | Added `monitor_tags_changed` message type | §0 #8 |
| Security | Credentials replace legacy secrets (dead `secrets.go`); `PULSE_SECRET_KEY` format; `combinedAuth` location; rotate-key gap (TD-008) | §0 #10, #12 |
| Backend/Frontend Conventions | Exact versions → pointer to `go.mod`/`package.json`; SvelteKit 2; `rapid`; test libs; `pnpm install` prerequisite | §4, §2 |
| MCP Server | Entrypoint, 12 tools with read/write split, transports; co-change list kept verbatim (verified correct); MCP type-map files for new monitor types | §3 |
| "Current Progress" + "Completed (A–J/CI)" | **Replaced** by a Feature Inventory table pointing to `.kiro/specs/<name>/`, `docs/PRODUCT-ROADMAP.md`, `docs/TECHNICAL-DEBT.md` | §8 |
| Heartbeat | One "Planned — NOT implemented" paragraph + entry in future-scope list | §7 |
| New "Development Workflow" | Run-locally table (correct `make dev` vs `make dev-local`, `down -v` warning), per-area pre-finish checks mirroring PR CI, optional integration suites, recipes: migration+sqlc, new checker/protocol (all touchpoints verified by grep for `"quic"`), new endpoint | §2, §6 |
| Build and Test | Replaced by "Commands Reference" listing all 13 root targets, 4 `mcp/Makefile` targets, 6 pnpm scripts with real behavior (`make openapi` = prints version) | §0 #1-3, §2 |
| Env vars | Two partial tables → **one** table of 26 vars (server, tooling, MCP) with code defaults (`PULSE_NOTIFICATION_WORKERS`=10, `DATABASE_URL` has default) | §0 #4-6, §1 |
| Infrastructure | Compose/Dockerfile summary + all 6 workflows with triggers | §6 |
| Delivery Constraints | Removed "alert channels"; added heartbeat as future scope | §0 #7 |
| Notifications | Collapsed 7 subsections into bullets; endpoint table replaced by route groups + pointer to OpenAPI; added `/monitors/{id}/delivery-logs`; reminders marked not wired (TD-004); TD-003/TD-005 noted; removed duplicated env table | §0 #11, §3 |
| i18n | Fixed command to `cd frontend && pnpm validate-locales`; documented the script's warn-only behavior and missing `ar`/`be`; new-language step to update the script list; heading number removed (points to `config.ts`) | §0 #3, §5 |
| Key Files | Removed dead `docs/MILESTONES.md` link; grouped rows; added config, middleware, handlers, crypto/token, proto/retention/tags, notify.go, dynamic_metrics.go, cmd/migrate+rotate, sqlc.yaml+queries, embed, MCP entry/config/register/tools/pulseapi, vite config, compose/Dockerfile/.env.example, workflows dir, docs, specs | §9 |
| New "Agent Tooling" | Skills (lock + registry, gitignored dir), Kiro hooks, Copilot file marked stale; Svelte/Go skill guidance kept | §9 |

### Removed
- Milestone status table and A–J/CI changelogs (history now in `.kiro/specs/`).
- "218 frontend tests passing" and other volatile counts; exact patch versions; TimescaleDB 7-day-window claim; "Settings (secrets)".
- Duplicated notification env table and the 16-row notification endpoint table (contract lives in `openapi.yaml`).
- Link to nonexistent `docs/MILESTONES.md`. (The task brief asked to point to it; it was deleted in `da598da`, so `.kiro/specs/` and `docs/` are referenced instead.)

### Normative rules
All prior rules kept verbatim (API, WS, security, performance, backend/frontend conventions, theming, dark mode, i18n checklist, MCP co-change, breaking-change rule). Rules corrected only where facts were wrong: validate-locales command, WS message types list, future-scope list. One rule added: "When adding an env var, update this table, `.env.example`, and README."

### Verification run
1. Extracted 110 backticked path-like tokens; all exist (`test -e`) except intentional templates/globs: `.kiro/specs/<name>/`, `backend/**`, `frontend/**`, `mcp/**`, `backend/internal/...`, `backend/internal/monitor/<type>.go`, `backend/migrations/NNN_name.up.sql`, `frontend/src/locales/{code}.json`, `backend/internal/store/postgres/queries/*.sql` (glob resolves). URL/route/package tokens excluded.
2. All 13 root make targets and 4 `mcp/Makefile` targets exist; all 6 pnpm scripts exist in `frontend/package.json`.
3. Versions: backend `go 1.26.0`, gin v1.12.0, pgx v5.10.0, jwt v5.3.1, migrate v4.19.1, rapid v1.3.0 (both modules); Svelte ^5.57, SvelteKit ^2.70, Tailwind ^3.4.19, TS `strict: true`; compose `golang:1.26`, `node:lts-alpine`, `timescaledb:2.28.3-pg17`; Dockerfile `node:lts-alpine` → `golang:1.26-alpine` → distroless.
4. Env: every var named in AGENTS.md is read by code; every `PULSE_*`/`DATABASE_URL`/`MIGRATIONS_PATH` string in Go code is in the table except `PULSE_TIMEOUT`/`PULSE_UNAUTHORIZED`/`PULSE_UNREACHABLE`, which are MCP error codes (`mcp/internal/mcperr/mcperr.go`), not env vars. Defaults re-read in `config.go` and `main.go` (notification workers 10, drain 30s).

### Staleness in other docs (listed, not edited)
See §10: `.github/copilot-instructions.md`, `README.md` (worker defaults, missing vars, QUIC), `docs/DEVELOPMENT.md` (`make dev`/`dev-local` reversed, postgres port, CI MCP job, prerequisites), `docs/API.md` (secrets endpoints), `.env.example`, `docker-compose.yml` placeholder secret key, `openapi.yaml` drift (3 routes), `router.go` secrets comment, `validate-locales.ts` locale list, `.atl/skill-registry.md` paths.

### New observation during this iteration
- HEAD is now `460cf50` on branch `feat/heartbeat-monitoring` (spec committed), and a second worktree `.worktrees/heartbeat` (`feat/heartbeat-impl`, same commit, clean) exists and is untracked/not gitignored in this tree. Heartbeat is still 0/36 tasks with no code; AGENTS.md wording holds. If implementation lands, AGENTS.md must be updated in that change (the spec's design.md already plans this).

### Open questions for the user (still open)
1. Heartbeat: kept as a one-line "Planned — NOT implemented" note + future-scope entry. Remove it entirely until implementation starts?
2. Linking `docs/PRODUCT-ROADMAP.md` / `docs/TECHNICAL-DEBT.md`: they are untracked in this tree. AGENTS.md links them; they must be committed together with AGENTS.md or the links dangle on other clones.
3. Should `.worktrees/` be gitignored?
4. Follow-up task for out-of-scope fixes (§10, OpenAPI drift, validate-locales list, CI tag overlap)?
5. `make openapi`: keep as "prints version", or add a real validator later?
