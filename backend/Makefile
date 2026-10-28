.PHONY: help run db test test-integration cover build lint tidy

TEST_DATABASE_URL ?= postgres://disputes:disputes@localhost:5432/disputes?sslmode=disable

help: ## Show available commands
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-18s %s\n", $$1, $$2}'

db: ## Start only Postgres (from the repo root compose file)
	docker compose -f ../docker-compose.yml up -d postgres

run: ## Run the API locally (needs Postgres: make db)
	set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/api

test: ## Unit tests with race detector, no cache
	go test -race -count=1 ./...

test-integration: ## Unit + integration tests against local Postgres (make db first)
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -count=1 ./...

cover: ## Coverage report
	go test -count=1 -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

build: ## Static production binary
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api

lint: ## golangci-lint
	golangci-lint run

tidy: ## Sync go.mod and go.sum
	go mod tidy
