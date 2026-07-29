.DEFAULT_GOAL := help

.PHONY: help up up-legacy down down-v ps logs db backend frontend install test test-backend test-frontend typecheck-frontend build-frontend vet fmt lint lint-backend lint-frontend ci

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## --- Docker (db + app + frontend) ---

up: ## Build and start Postgres + the Go app + the frontend in Docker
	docker compose up -d --build

up-legacy: ## Same as `up`, but with buildkit disabled (fixes some WSL/Docker Desktop bind-mount errors)
	DOCKER_BUILDKIT=0 docker compose up -d --build

down: ## Stop the Docker stack (keeps data)
	docker compose down

down-v: ## Stop the Docker stack and delete the Postgres volume
	docker compose down -v

db: ## Start only Postgres (for local `make backend` dev)
	docker compose up -d db

ps: ## Show status of the Docker stack
	docker compose ps

logs: ## Follow logs from the Docker stack
	docker compose logs -f

## --- Local dev (outside Docker) ---

backend: ## Run the Go API locally against `make db` (listens on :3000)
	cd backend && go run ./cmd/api

install: ## Install frontend dependencies
	cd frontend && npm install

frontend: ## Run the Vite dev server (http://localhost:5173, proxies /api -> :3000)
	cd frontend && npm run dev

## --- Checks ---

test: test-backend test-frontend ## Run all tests

test-backend: ## Run Go unit tests (fake repository, no DB needed)
	cd backend && go test ./...

test-frontend: ## Run frontend unit tests (Vitest)
	cd frontend && npm test

vet: ## Run go vet
	cd backend && go vet ./...

fmt: ## Run gofmt on the backend
	cd backend && gofmt -l -w .

lint: lint-backend lint-frontend ## Run all linters

lint-backend: ## Run golangci-lint on the backend
	cd backend && golangci-lint run ./...

lint-frontend: ## Run oxlint (TS/JS linter) on the frontend
	cd frontend && npm run lint

typecheck-frontend: ## Typecheck the frontend
	cd frontend && npx tsc -b --noEmit

build-frontend: ## Production build the frontend
	cd frontend && npm run build

ci: vet lint test typecheck-frontend build-frontend ## Run everything CI runs, locally
