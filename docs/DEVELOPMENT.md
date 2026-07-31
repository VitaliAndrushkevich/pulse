# Development Guide

## Prerequisites

- Go 1.26+
- Node.js 22+
- pnpm 9+ (install via `corepack enable` or [pnpm.io/installation](https://pnpm.io/installation))
- PostgreSQL 16 with TimescaleDB 2.17+
- Make

## Make Targets

| Target | Description |
|--------|-------------|
| `make dev` | Full stack via Docker Compose (Pulse + TimescaleDB) |
| `make dev-local` | Lightweight compose (backend + postgres only) |
| `make run` | `go run ./cmd/pulse` (requires local postgres) |
| `make build` | Build Go binary |
| `make build-frontend` | Build frontend and copy to embed path |
| `make build-all` | Production build: frontend + Go binary with embedded assets |
| `make test` | Run Go tests (`go test ./...`) |
| `make migrate` | Run database migrations up |
| `make migrate-down` | Roll back last migration |
| `make rotate-key` | AES key rotation with transactional re-encryption |
| `make openapi` | Validate OpenAPI spec |

## Local Setup

```bash
# Start database
docker compose up postgres -d

# Run migrations
make migrate

# Start backend (hot-reload with go run)
make run

# In a separate terminal — start frontend dev server
cd frontend && pnpm install && pnpm dev
```

### Frontend Dev Container

For containerized frontend development with hot module replacement (HMR), use the `frontend` service in `docker-compose.dev.yml`:

```bash
docker compose -f docker-compose.dev.yml up frontend
```

This starts the Vite dev server on port **5173** with HMR enabled — source file changes are reflected in the browser instantly.

## Running Tests

```bash
# Backend tests
make test

# Frontend tests (unit + property-based via fast-check)
cd frontend && pnpm test
```

## CI Pipeline

Pull requests to `main` trigger automated checks via GitHub Actions (`.github/workflows/pull_request_opened.yml`):

- **Backend**: Go tests with race detector against a real TimescaleDB service, plus binary build verification
- **Frontend**: TypeScript type check, locale validation, Vitest unit tests, production build

PRs in the same concurrency group cancel previous runs automatically.

## Release Pipeline

Pushing a semver tag (`v*`) triggers the release workflow (`.github/workflows/release.yml`):

1. **Build binaries** — Cross-compiles `pulse` and `pulse-mcp` for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 (CGO disabled, stripped)
2. **Docker image** — Builds and pushes a multi-arch image (linux/amd64 + arm64) to `ghcr.io` with semver tags (`v1.2.3`, `v1.2`, `v1`)
3. **GitHub Release** — Attaches all binaries and a SHA-256 checksums file to the release

To create a release:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Brand Assets

Logo files and brand guidelines are in `frontend/static/brand/`. To regenerate PNG exports from the SVG source:

```bash
cd frontend && node scripts/generate-brand-pngs.mjs
```

To regenerate favicon and PWA icons:

```bash
cd frontend && node scripts/generate-icons.mjs
```
