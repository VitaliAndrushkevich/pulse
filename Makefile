.PHONY: dev dev-down run build test migrate migrate-down rotate-key openapi

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
	cd backend && go run ./cmd/pulse

build:
	cd backend && go build ./cmd/pulse

test:
	cd backend && go test ./...

migrate:
	cd backend && DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrate -direction up

migrate-down:
	cd backend && DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrate -direction down

rotate-key:
	cd backend && DATABASE_URL=$(DATABASE_URL) go run ./cmd/rotate

openapi:
	@echo "not implemented yet: openapi generation"
