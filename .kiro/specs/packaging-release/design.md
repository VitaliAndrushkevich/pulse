# Design: Packaging & Release (Milestone H)

## Overview
This design covers how to embed the frontend into the Go binary, build a minimal container image, harden the compose setup, document the project, and add CI.

## 1. Static Embedding (TASK-036)

### Approach
- Create `backend/internal/frontend/frontend.go` with `go:embed` directive pointing to a `dist` directory
- The actual frontend build output is copied into this directory before `go build`
- In the Dockerfile, the Node build stage output is copied to `backend/internal/frontend/dist/` before the Go build stage
- For local development without embedding, use a build tag or check if embed is empty

### SPA Routing
- Register a `NoRoute` handler on the gin engine
- If the request path starts with `/api/`, `/ws`, `/metrics`, `/healthz`, or `/swagger` → return 404 JSON
- Otherwise → serve `index.html` from the embedded filesystem (SPA catch-all)
- Static assets (JS, CSS, images) served directly from embedded FS with content-type detection

### File Structure
```
backend/internal/frontend/
├── frontend.go      # go:embed directive, ServeEmbedded() function
└── dist/            # .gitignored, populated at build time
    └── (frontend build output)
```

## 2. Multi-Stage Dockerfile (TASK-037)

### Stages
1. **node-builder**: `node:22-alpine`, installs deps, runs `npm run build`
2. **go-builder**: `golang:1.25-alpine`, copies frontend build into embed path, runs `go build`
3. **runtime**: `gcr.io/distroless/static-debian12`, copies binary only

### Key Decisions
- Alpine for build stages (smaller download, faster CI)
- Distroless for runtime (no shell attack surface)
- Single binary with embedded assets = no volume mounts needed for frontend

## 3. Compose Hardening (TASK-038)

### Changes to docker-compose.yml
- Add health check for pulse service (`/healthz` endpoint)
- Set `PULSE_DEV=false` in production compose
- Add resource limits (memory)
- Document all env vars with comments
- Add `.env.example` file

## 4. README (TASK-039)

### Structure
- Project description and architecture overview
- Prerequisites (Docker, Docker Compose)
- Quick Start (3-step: clone, configure .env, docker compose up)
- Environment variables table
- API usage examples (login, create monitor, check status)
- Development setup (make targets)
- Architecture diagram (text-based)

## 5. CI Pipeline (TASK-040)

### GitHub Actions: `.github/workflows/ci.yml`
- Trigger: push to main, PR to main
- Jobs:
  - `lint`: golangci-lint
  - `test-backend`: go test with postgres service container
  - `test-frontend`: npm test in frontend/
  - `build`: docker build (proves image builds successfully)
  - `openapi-check`: verify spec is committed and valid
