.PHONY: dev dev-down run build test migrate rotate-key openapi

COMPOSE ?= docker compose
COMPOSE_DEV ?= docker compose -f docker-compose.dev.yml

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
	@echo "migration wiring pending (Phase 1 TASK-004): use backend/migrations with golang-migrate"

rotate-key:
	@echo "not implemented yet: key rotation workflow"

openapi:
	@echo "not implemented yet: openapi generation"
