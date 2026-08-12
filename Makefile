# Reddit Cluster Map - Root Makefile
# Manages both backend and frontend development workflows

# Environment - gracefully handle missing .env
-include backend/.env
export

# Paths (absolute to avoid duplication issues when changing directories)
ROOT_DIR := $(CURDIR)
COMPOSE_FILE_PATH := $(ROOT_DIR)/backend/docker-compose.yml
COMPOSE_TEST_FILE_PATH := $(ROOT_DIR)/backend/docker-compose.test.yml
MIGRATIONS_DIR := $(ROOT_DIR)/backend/migrations
MIGRATE_IMAGE := migrate/migrate:latest

# Docker container names
DB_CONTAINER = reddit-cluster-db
API_CONTAINER = reddit-cluster-api
CRAWLER_CONTAINER = reddit-cluster-crawler
FRONTEND_CONTAINER = reddit-cluster-frontend

# Database credentials
DB_USER = $(POSTGRES_USER)
DB_NAME = $(POSTGRES_DB)

.PHONY: help setup check-env check-tools install-tools

# Default target - show help
.DEFAULT_GOAL := help

# Help target - shows all available targets with descriptions
help: ## Show this help message
	@echo "Reddit Cluster Map - Development Commands"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "\033[36m%-30s\033[0m %s\n", "Target", "Description"} /^[a-zA-Z_-]+:.*?##/ { printf "\033[36m%-30s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Setup

setup: ## Initial setup - install dependencies and configure environment
	@echo "==> Setting up Reddit Cluster Map..."
	@if [ ! -f backend/.env ]; then \
		echo "Creating backend/.env from backend/.env.example..."; \
		cp backend/.env.example backend/.env 2>/dev/null || echo "⚠️  backend/.env.example not found"; \
		echo "✓ Created backend/.env - please edit it with your configuration"; \
		echo "  Required: REDDIT_CLIENT_ID, REDDIT_CLIENT_SECRET, POSTGRES_PASSWORD"; \
	else \
		echo "✓ backend/.env already exists"; \
	fi
	@echo ""
	@$(MAKE) check-tools
	@echo ""
	@echo "==> Installing backend dependencies..."
	@cd backend && go mod download
	@echo "✓ Backend dependencies installed"
	@echo ""
	@echo "==> Installing frontend dependencies..."
	@cd frontend && npm install
	@echo "✓ Frontend dependencies installed"
	@echo ""
	@echo "✓ Setup complete! Next steps:"
	@echo "  1. Edit backend/.env with your Reddit API credentials"
	@echo "  2. Run 'make up' to start all services"
	@echo "  3. Run 'make migrate-up' to initialize the database"

check-env: ## Check if .env file exists and is configured
	@if [ ! -f backend/.env ]; then \
		echo "❌ Error: backend/.env file not found"; \
		echo "Run 'make setup' to create it from backend/.env.example"; \
		exit 1; \
	fi
	@if grep -q "your_client_id_here" backend/.env 2>/dev/null || grep -q "change_me_in_production" backend/.env 2>/dev/null; then \
		echo "⚠️  Warning: backend/.env contains example values. Please configure:"; \
		echo "  - REDDIT_CLIENT_ID"; \
		echo "  - REDDIT_CLIENT_SECRET"; \
		echo "  - POSTGRES_PASSWORD"; \
	fi

check-tools: ## Check if required tools are installed
	@echo "Checking required tools..."
	@command -v docker >/dev/null 2>&1 || { echo "❌ docker not found. Install from https://docs.docker.com/get-docker/"; exit 1; }
	@echo "✓ docker"
	@command -v docker compose version >/dev/null 2>&1 || { echo "❌ docker compose not found. Install from https://docs.docker.com/compose/install/"; exit 1; }
	@echo "✓ docker compose"
	@command -v go >/dev/null 2>&1 || { echo "⚠️  go not found (optional for local dev). Install from https://go.dev/doc/install"; }
	@command -v go >/dev/null 2>&1 && echo "✓ go $$(go version | awk '{print $$3}')" || true
	@command -v node >/dev/null 2>&1 || { echo "⚠️  node not found (optional for local dev). Install from https://nodejs.org/"; }
	@command -v node >/dev/null 2>&1 && echo "✓ node $$(node --version)" || true
	@command -v npm >/dev/null 2>&1 && echo "✓ npm $$(npm --version)" || true
	@command -v sqlc >/dev/null 2>&1 && echo "✓ sqlc" || echo "⚠️  sqlc not found (needed for 'make generate'). Run 'make install-tools'"
	@command -v migrate >/dev/null 2>&1 && echo "✓ migrate" || echo "⚠️  migrate not found (needed for migrations). Run 'make install-tools'"

install-tools: ## Install sqlc and golang-migrate (requires Go)
	@command -v go >/dev/null 2>&1 || { echo "❌ go not found. Install Go first from https://go.dev/doc/install"; exit 1; }
	@echo "Installing sqlc..."
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@echo "Installing golang-migrate..."
	@go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "✓ Tools installed to $(shell go env GOPATH)/bin"
	@echo "  Make sure $(shell go env GOPATH)/bin is in your PATH"

##@ Docker Services

up: check-env ## Start all services (database, API, crawler, frontend, monitoring)
	@echo "==> Starting all services..."
	@docker compose -f $(COMPOSE_FILE_PATH) up -d
	@echo "✓ Services started"
	@echo ""
	@echo "Services available at:"
	@echo "  Frontend:   http://localhost (or configured port)"
	@echo "  API:        http://localhost:8000"
	@echo "  Grafana:    http://localhost:3000"
	@echo "  Prometheus: http://localhost:9090"

down: ## Stop all services
	@echo "==> Stopping all services..."
	@docker compose -f $(COMPOSE_FILE_PATH) down
	@echo "✓ Services stopped"

restart: ## Restart all services
	@$(MAKE) down
	@$(MAKE) up

rebuild: ## Rebuild and restart all services
	@echo "==> Rebuilding all services..."
	@docker compose -f $(COMPOSE_FILE_PATH) up -d --build
	@echo "✓ Services rebuilt and started"

ps: ## Show running containers
	@docker compose -f $(COMPOSE_FILE_PATH) ps

logs: ## Follow logs from all services
	@docker compose -f $(COMPOSE_FILE_PATH) logs -f

logs-api: ## Follow API server logs
	@docker compose -f $(COMPOSE_FILE_PATH) logs -f api

logs-db: ## Follow database logs
	@docker compose -f $(COMPOSE_FILE_PATH) logs -f db

logs-crawler: ## Follow crawler logs
	@docker compose -f $(COMPOSE_FILE_PATH) logs -f crawler

logs-frontend: ## Follow frontend logs
	@docker compose -f $(COMPOSE_FILE_PATH) logs -f reddit_frontend

##@ Database

# Container-based migrations (preferred)
migrate-up: check-env ## Run database migrations inside container network
	@echo "==> Running migrations in container (network: web, host: db)"
	@docker run --rm \
		--network web \
		--env-file backend/.env \
		-v $(MIGRATIONS_DIR):/migrations:ro \
		$(MIGRATE_IMAGE) \
		-path=/migrations -database "$$DATABASE_URL" up
	@echo "✓ Migrations complete"

migrate-down: check-env ## Rollback last migration inside container network
	@echo "==> Rolling back last migration in container"
	@docker run --rm \
		--network web \
		--env-file backend/.env \
		-v $(MIGRATIONS_DIR):/migrations:ro \
		$(MIGRATE_IMAGE) \
		-path=/migrations -database "$$DATABASE_URL" down 1

# Host-based migrations (optional)
migrate-up-host: check-env ## Run database migrations from host to localhost:5432
	@command -v migrate >/dev/null 2>&1 || { echo "❌ migrate not found. Run 'make install-tools'"; exit 1; }
	@echo "Running migrations (localhost)..."
	@env -u DATABASE_URL migrate -path backend/migrations -database "postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(POSTGRES_DB)?sslmode=disable" up || { echo "❌ Migration failed"; exit 1; }
	@echo "✓ Migrations complete"

migrate-down-host: check-env ## Rollback last migration from host to localhost:5432
	@command -v migrate >/dev/null 2>&1 || { echo "❌ migrate not found. Run 'make install-tools'"; exit 1; }
	@echo "Rolling back last migration (localhost)..."
	@env -u DATABASE_URL migrate -path backend/migrations -database "postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(POSTGRES_DB)?sslmode=disable" down 1

db-reset: check-env ## Reset database (dangerous - drops all data)
	@echo "⚠️  This will delete all data. Press Ctrl+C to cancel, or press Enter to continue..."
	@read confirm
	@cd backend && docker compose down -v
	@cd backend && docker compose up -d db
	@sleep 3
	@$(MAKE) migrate-up
	@echo "✓ Database reset complete"

db-shell: ## Open psql shell to database
	@docker exec -it $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME)

##@ Development - Backend

dev-backend: ## Run backend API locally (requires DATABASE_URL)
	@cd backend && go run ./cmd/server

test-backend: ## Run backend tests
	@echo "==> Running backend tests..."
	@cd backend && go test ./...
	@echo "✓ Backend tests passed"

test-backend-verbose: ## Run backend tests with verbose output
	@cd backend && go test -v ./...

test-backend-coverage: ## Run backend tests with coverage
	@cd backend && go test -v -race -coverprofile=coverage.out ./...
	@cd backend && go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report generated: backend/coverage.html"

test-integration: ## Run integration tests (requires database)
	@echo "==> Running integration tests..."
	@cd backend && go test ./internal/graph -run Integration -v

test-clone-pipeline: ## Validate production database-clone shell contracts
	@bash -n scripts/clone/*.sh
	@scripts/clone/test-validation.sh

lint-backend: ## Run Go linters
	@echo "Running go vet..."
	@cd backend && go vet ./...
	@echo "✓ go vet passed"
	@echo ""
	@echo "Checking gofmt..."
	@if [ -n "$$(cd backend && gofmt -l .)" ]; then \
		echo "❌ The following files need formatting:"; \
		cd backend && gofmt -l .; \
		echo ""; \
		echo "Run 'make fmt-backend' to fix formatting"; \
		exit 1; \
	fi
	@echo "✓ gofmt check passed"

fmt-backend: ## Format backend Go code
	@echo "Formatting Go code..."
	@cd backend && gofmt -w .
	@echo "✓ Code formatted"

##@ Development - Frontend

dev-frontend: ## Run frontend dev server locally
	@echo "==> Starting frontend dev server..."
	@cd frontend && npm run dev

build-frontend: ## Build frontend for production
	@echo "==> Building frontend..."
	@cd frontend && npm run build
	@echo "✓ Frontend built to frontend/dist"

test-frontend: ## Run frontend tests
	@echo "==> Running frontend tests..."
	@cd frontend && npm test
	@echo "✓ Frontend tests passed"

test-frontend-ui: ## Run frontend tests with UI
	@cd frontend && npm run test:ui

lint-frontend: ## Run frontend linter
	@echo "==> Running frontend linter..."
	@cd frontend && npm run lint

fmt-frontend: ## Format frontend code (if configured)
	@cd frontend && npm run format 2>/dev/null || echo "⚠️  No format script configured"

##@ Testing

test: test-backend test-frontend ## Run all tests (backend + frontend)

test-all: test ## Alias for 'test'

lint: lint-backend lint-frontend ## Run all linters

fmt: fmt-backend ## Format all code

##@ Code Generation

generate: ## Generate code (sqlc)
	@command -v sqlc >/dev/null 2>&1 || { echo "❌ sqlc not found. Run 'make install-tools'"; exit 1; }
	@echo "==> Generating sqlc code..."
	@cd backend && sqlc generate
	@echo "✓ Code generated"

##@ Crawling & Data

crawl: check-env ## Start a crawl job (requires SUB=subreddit_name)
	@if [ -z "$(SUB)" ]; then \
		echo "❌ Error: SUB variable not set"; \
		echo "Usage: make crawl SUB=AskReddit"; \
		exit 1; \
	fi
	@echo "==> Starting crawl for r/$(SUB)..."
	@curl -X POST http://localhost:8000/api/crawl \
	  -H "Content-Type: application/json" \
	  -d '{"subreddit": "$(SUB)"}'
	@echo ""

precalculate: check-env ## Run graph precalculation
	@echo "==> Running graph precalculation..."
	@cd backend && docker compose run --rm precalculate /app/precalculate
	@echo "✓ Precalculation complete"

##@ Deployment

deploy: ## Rebuild services and run migrations
	@echo "==> Deploying: rebuild + migrations"
	@$(MAKE) rebuild
	@sleep 2
	@$(MAKE) migrate-up
	@echo "✓ Deploy complete"

deploy-build: ## Build images and start services
	@echo "==> Building and starting services"
	@docker compose -f $(COMPOSE_FILE_PATH) build
	@docker compose -f $(COMPOSE_FILE_PATH) up -d
	@echo "✓ Services built and started"

##@ Migrations - Utilities

migrate-status: ## Show migration version/status (container)
	@echo "==> Migration status"
	@docker run --rm \
		--network web \
		--env-file backend/.env \
		-v $(MIGRATIONS_DIR):/migrations:ro \
		$(MIGRATE_IMAGE) \
		-path=/migrations -database "$$DATABASE_URL" version

##@ Backups

backup-now: check-env ## Create a database backup
	@echo "==> Creating database backup..."
	@cd backend && docker compose run --rm --no-deps \
		-e PGHOST=$${PGHOST:-db} \
		-e POSTGRES_USER=$${POSTGRES_USER} \
		-e POSTGRES_PASSWORD=$${POSTGRES_PASSWORD} \
		-e POSTGRES_DB=$${POSTGRES_DB} \
		precalculate /app/scripts/backup.sh
	@echo "✓ Backup complete"

backups-list: ## List available backups
	@docker run --rm -v reddit-cluster-pgbackups:/data busybox sh -c 'ls -lh /data | sort -k9'

backups-download: ## Download latest backup to ./backups/
	@mkdir -p backups
	@docker run --rm -v reddit-cluster-pgbackups:/data -v $$PWD/backups:/out busybox /bin/sh -c \
		"set -e; sel=; for f in \$$(ls -1t /data 2>/dev/null || true); do \
			if [ -s \"/data/\$$f\" ]; then sel=\"\$$f\"; break; fi; \
		done; \
		if [ -n \"\$$sel\" ]; then echo \"Copying: \$$sel\"; cp -v /data/\"\$$sel\" /out/; else echo 'No backups found'; fi"

##@ Monitoring

monitoring-up: ## Start monitoring stack (Prometheus + Grafana)
	@echo "==> Starting monitoring services..."
	@cd backend && docker compose up -d prometheus grafana
	@echo "✓ Monitoring services started"
	@echo "  Grafana:    http://localhost:3000"
	@echo "  Prometheus: http://localhost:9090"

monitoring-down: ## Stop monitoring stack
	@cd backend && docker compose stop prometheus grafana

##@ Load Testing

loadtest-setup: check-env ## Setup load testing environment
	@echo "==> Setting up load testing environment..."
	@docker compose -f $(COMPOSE_FILE_PATH) -f $(ROOT_DIR)/backend/docker-compose.loadtest.yml up -d k6
	@echo "✓ Load testing environment ready"

loadtest-teardown: ## Teardown load testing environment
	@echo "==> Tearing down load testing environment..."
	@docker compose -f $(COMPOSE_FILE_PATH) -f $(ROOT_DIR)/backend/docker-compose.loadtest.yml down k6
	@echo "✓ Load testing environment removed"

loadtest-smoke: check-env ## Run smoke test (quick sanity check)
	@echo "==> Running smoke test (1 VU, 30s)..."
	@docker compose -f $(COMPOSE_FILE_PATH) -f $(ROOT_DIR)/backend/docker-compose.loadtest.yml run --rm k6 run /scripts/smoke.js
	@echo "✓ Smoke test complete"

loadtest-load: check-env ## Run load test (50 VU, 5min)
	@echo "==> Running load test (50 VU, 5min)..."
	@echo "⚠️  This test will take approximately 5 minutes to complete"
	@docker compose -f $(COMPOSE_FILE_PATH) -f $(ROOT_DIR)/backend/docker-compose.loadtest.yml run --rm k6 run /scripts/load.js
	@echo "✓ Load test complete"

loadtest-stress: check-env ## Run stress test (200 VU ramp, 2min)
	@echo "==> Running stress test (200 VU ramp, 2min)..."
	@echo "⚠️  This test will push the system to its limits"
	@docker compose -f $(COMPOSE_FILE_PATH) -f $(ROOT_DIR)/backend/docker-compose.loadtest.yml run --rm k6 run /scripts/stress.js
	@echo "✓ Stress test complete"

loadtest-soak: check-env ## Run soak test (10 VU, 30min)
	@echo "==> Running soak test (10 VU, 30min)..."
	@echo "⚠️  This test will take approximately 30 minutes to complete"
	@echo "⚠️  Monitor memory usage in Grafana during this test"
	@docker compose -f $(COMPOSE_FILE_PATH) -f $(ROOT_DIR)/backend/docker-compose.loadtest.yml run --rm k6 run /scripts/soak.js
	@echo "✓ Soak test complete"

loadtest: ## Run all load tests (smoke, load, stress, soak)
	@echo ""
	@echo "==> Running full load test suite..."
	@echo "⚠️  This will take approximately 40 minutes to complete"
	@echo ""
	@$(MAKE) loadtest-smoke
	@echo ""
	@echo "Smoke test passed ✓"
	@echo "Waiting 10 seconds before load test..."
	@sleep 10
	@echo ""
	@$(MAKE) loadtest-load
	@echo ""
	@echo "Load test complete ✓"
	@echo "Waiting 30 seconds before stress test..."
	@sleep 30
	@echo ""
	@$(MAKE) loadtest-stress
	@echo ""
	@echo "Stress test complete ✓"
	@echo "Waiting 60 seconds before soak test..."
	@sleep 60
	@echo ""
	@$(MAKE) loadtest-soak
	@echo ""
	@echo "✓ All load tests complete!"
	@echo ""
	@echo "Results saved to backend/loadtest/results/"
	@echo "Review results and compare with baselines in backend/loadtest/README.md"

loadtest-results: ## View latest load test results
	@echo "==> Latest load test results:"
	@ls -lth $(ROOT_DIR)/backend/loadtest/results/*.json 2>/dev/null | head -5 || echo "No results found"

##@ Maintenance

clean: ## Clean up build artifacts and temporary files
	@echo "==> Cleaning up..."
	@cd backend && rm -f coverage.out coverage.html *.prof
	@cd frontend && rm -rf dist node_modules/.cache
	@echo "✓ Cleanup complete"

clean-volumes: ## Remove all Docker volumes (dangerous - deletes all data)
	@echo "⚠️  This will delete all Docker volumes including database data!"
	@echo "Press Ctrl+C to cancel, or press Enter to continue..."
	@read confirm
	@cd backend && docker compose down -v
	@echo "✓ Volumes removed"

prune: ## Prune Docker system (removes unused containers, networks, images)
	@echo "==> Pruning Docker system..."
	@docker system prune -f
	@echo "✓ Docker system pruned"

##@ Quick Start

quickstart: setup up migrate-up ## Quick start - setup, start services, and run migrations
	@echo ""
	@echo "✓ Reddit Cluster Map is ready!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Start a crawl: make crawl SUB=AskReddit"
	@echo "  2. Run precalculation: make precalculate"
	@echo "  3. Open frontend: http://localhost"
	@echo "  4. Check API health: curl http://localhost:8000/health"

status: ## Show status of all services
	@echo "==> Service Status"
	@cd backend && docker compose ps
	@echo ""
	@echo "==> Database Connection"
	@docker exec $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -c "SELECT version();" 2>/dev/null && echo "✓ Database accessible" || echo "❌ Database not accessible"
	@echo ""
	@echo "==> API Health"
	@curl -s http://localhost:8000/health 2>/dev/null | jq . || echo "❌ API not accessible"
