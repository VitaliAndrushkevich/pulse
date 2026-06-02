.PHONY: dev dev-down run build build-frontend build-all test migrate migrate-down rotate-key openapi

COMPOSE ?= docker compose
COMPOSE_DEV ?= docker compose -f docker-compose.dev.yml

# Default DATABASE_URL for local development (matches docker-compose postgres service).
DATABASE_URL ?= postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable

dev:
	$(COMPOSE) up --build

dev-local:
	$(COMPOSE_DEV) up --build

dev-down:
	$(COMPOSE) down -v

dev-local-down:
	$(COMPOSE_DEV) down -v

run:
	cd backend && \
		PULSE_PORT=8080 \
		PULSE_DEV=true \
		PULSE_SCHEDULER_WORKERS=50 \
		PULSE_SECRET_KEY="cHVsc2UtZGV2LXNlY3JldC1rZXktMDEyMzQ1Njc4OTA=" \
		PULSE_JWT_SECRET="pulse-dev-jwt-secret-change-in-production" \
		PULSE_JWT_EXPIRY=24h \
		DATABASE_URL="$(DATABASE_URL)" \
		go run ./cmd/pulse

build:
	cd backend && go build ./cmd/pulse

# Build frontend and copy output to the Go embed path.
build-frontend:
	cd frontend && npm run build
	rm -rf backend/internal/frontend/dist
	mkdir -p backend/internal/frontend/dist
	cp -r frontend/build/* backend/internal/frontend/dist/
	cp backend/internal/frontend/dist/.gitkeep backend/internal/frontend/dist/.gitkeep 2>/dev/null || touch backend/internal/frontend/dist/.gitkeep

# Production build: builds frontend, embeds assets, then compiles Go binary.
build-all: build-frontend
	cd backend && go build -o pulse ./cmd/pulse

test:
	cd backend && go test ./...

migrate:
	cd backend && DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrate -direction up

migrate-down:
	cd backend && DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrate -direction down

rotate-key:
	cd backend && DATABASE_URL=$(DATABASE_URL) go run ./cmd/rotate

openapi:
	@echo "OpenAPI spec located at backend/api/openapi.yaml"
	@echo "Validating spec..."
	@if command -v yq > /dev/null 2>&1; then \
		yq eval '.info.version' backend/api/openapi.yaml; \
	else \
		echo "  (install yq for validation)"; \
	fi
	@echo "Done."
