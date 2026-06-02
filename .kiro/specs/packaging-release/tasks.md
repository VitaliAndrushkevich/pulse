# Implementation Plan: Packaging & Release (Milestone H)

## Overview

This plan packages Pulse into a single deployable container with embedded frontend, production-ready Docker Compose, documentation, and CI. Tasks are ordered by dependency: embedding → Docker image → compose hardening → docs → CI.

## Tasks

- [x] 1. Static embedding and SPA serving
  - [x] 1.1 Create frontend embed package
    - Create `backend/internal/frontend/frontend.go` with `//go:embed dist/*` directive
    - Create `backend/internal/frontend/dist/.gitkeep` placeholder (actual build output is .gitignored)
    - Add `backend/internal/frontend/dist/` to `.gitignore` (except .gitkeep)
    - Export `FS` variable (embed.FS) and a `HasAssets()` helper to check if embed is populated
    - _Requirements: 1.1_

  - [x] 1.2 Add SPA catch-all route to gin router
    - In `backend/internal/api/router.go`, add a `NoRoute` handler
    - If request path matches API/system prefixes (`/api/`, `/ws`, `/metrics`, `/healthz`, `/swagger`) → return standard 404 JSON error
    - Otherwise → serve from embedded frontend FS: try exact file first, fall back to `index.html`
    - Serve static assets with proper MIME types using `http.FileServer` over the embed.FS
    - Add `Cache-Control` headers: immutable for hashed assets (`.js`, `.css` with hash in filename), no-cache for `index.html`
    - Only register SPA routes when `frontend.HasAssets()` returns true (dev mode without build still works)
    - _Requirements: 1.2, 1.3, 1.4_

  - [x] 1.3 Add build script to copy frontend output into embed path
    - Add a `Makefile` target `build-frontend` that runs `npm run build` in `frontend/` and copies output to `backend/internal/frontend/dist/`
    - Add a `build-all` target that runs `build-frontend` then `go build`
    - Update existing `build` target to mention `build-all` for production builds
    - _Requirements: 1.1, 1.2_

- [x] 2. Multi-stage Docker image
  - [x] 2.1 Rewrite Dockerfile with three stages
    - Stage 1 (`node-builder`): `node:22-alpine`, copy `frontend/`, install deps, run `npm run build`
    - Stage 2 (`go-builder`): `golang:1.25-alpine`, copy `backend/`, copy frontend build output to `backend/internal/frontend/dist/`, run `CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/pulse ./cmd/pulse`
    - Stage 3 (`runtime`): `gcr.io/distroless/static-debian12`, copy binary only, expose 8080, set entrypoint
    - No need to copy openapi.yaml separately since it's part of the Go source tree (embedded or accessed at build time)
    - _Requirements: 2.1, 2.2, 2.3, 2.4_

  - [x] 2.2 Add .dockerignore for efficient builds
    - Ignore `node_modules/`, `.git/`, `*.md` (except README), test files, IDE configs
    - Keep `frontend/`, `backend/`, `Makefile` in context
    - _Requirements: 2.4_

- [x] 3. Compose hardening
  - [x] 3.1 Update docker-compose.yml for production
    - Add health check for pulse service: `curl -f http://localhost:8080/healthz || exit 1` (use wget in distroless-compatible way or rely on Docker's built-in HTTP health check)
    - Since distroless has no shell/curl, use the `/healthz` endpoint with Docker's native healthcheck mechanism or add a tiny health binary
    - Set `PULSE_DEV: "false"` as default
    - Add `restart: unless-stopped` to pulse service
    - Add memory limits via `deploy.resources.limits`
    - Document env vars with inline comments
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

  - [x] 3.2 Create .env.example file
    - Include all required environment variables with safe placeholder values and descriptions
    - Reference from docker-compose.yml via `env_file` directive
    - _Requirements: 3.4_

- [x] 4. README quick start
  - [x] 4.1 Rewrite README.md with complete documentation
    - Project overview: what Pulse is, key features, architecture (single binary, embedded frontend, PostgreSQL+TimescaleDB)
    - Prerequisites: Docker, Docker Compose v2
    - Quick Start: 3 steps (clone, copy .env.example to .env, docker compose up)
    - Environment variables table with descriptions and defaults
    - API usage examples: login, create monitor, list monitors, check WebSocket
    - Development section: make targets, local setup, running tests
    - Architecture section: package layout, data flow
    - _Requirements: 4.1, 4.2, 4.3_

## Notes

- CI quality gates (GitHub Actions) deferred to a future iteration
- All packaging tasks are complete: the project now builds as a single container with embedded frontend

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "1.3"] },
    { "id": 2, "tasks": ["2.1", "2.2"] },
    { "id": 3, "tasks": ["3.1", "3.2"] },
    { "id": 4, "tasks": ["4.1"] }
  ]
}
```
