# =============================================================================
# Stage 1: Build Frontend
# =============================================================================
FROM node:22-alpine AS node-builder

WORKDIR /src/frontend
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/ ./
RUN pnpm run build

# =============================================================================
# Stage 2: Build Go Binary (with embedded frontend assets)
# =============================================================================
FROM golang:1.25-alpine AS go-builder

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

# Copy frontend build output into the embed path
COPY --from=node-builder /src/frontend/build/ ./internal/frontend/dist/

# Build a statically-linked binary with stripped debug symbols
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/pulse ./cmd/pulse

# =============================================================================
# Stage 3: Minimal Runtime (distroless — no shell, no package manager)
# =============================================================================
FROM gcr.io/distroless/static-debian12 AS runtime

WORKDIR /app
COPY --from=go-builder /out/pulse /app/pulse
COPY --from=go-builder /src/backend/api/ /app/api/

EXPOSE 8080
ENTRYPOINT ["/app/pulse"]
