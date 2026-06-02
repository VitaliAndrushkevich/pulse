# Requirements: Packaging & Release (Milestone H)

## Overview
Package Pulse into a single deployable container with embedded frontend assets, production-ready Docker Compose, documentation, and CI pipeline.

## Requirements

### 1. Static Embedding
- 1.1 Frontend static build output (`frontend/build`) must be embedded into the Go binary via `go:embed`
- 1.2 The Go binary must serve both the API (`/api/v1/*`, `/ws`, `/metrics`, `/healthz`) and the frontend SPA from a single process
- 1.3 SPA catch-all routing: any request not matching API/system routes must return `index.html` for client-side routing
- 1.4 Static assets must be served with appropriate cache headers

### 2. Container Image
- 2.1 Multi-stage Dockerfile: Node build stage (frontend) → Go build stage (backend with embedded assets) → distroless runtime
- 2.2 Final image must be minimal (distroless, no shell, no package manager)
- 2.3 Image must start and serve the full application (API + frontend) successfully
- 2.4 Image size should be under 50MB

### 3. Production Compose
- 3.1 Health checks on both pulse and postgres services
- 3.2 Production-safe defaults (non-dev mode, restart policies)
- 3.3 Named volumes for data persistence
- 3.4 Clear environment variable documentation

### 4. Documentation
- 4.1 README must document: prerequisites, quick start, environment variables, migration flow, basic API usage
- 4.2 New machine setup must work following only the README
- 4.3 Include example docker-compose override for customization

### 5. CI Pipeline
- 5.1 GitHub Actions workflow for PRs
- 5.2 Jobs: lint (golangci-lint), test (go test), build (Docker image), OpenAPI drift check
- 5.3 Pipeline must enforce contract and build integrity
- 5.4 Frontend tests included in CI
