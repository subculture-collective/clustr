# Developer Guide

This guide covers development workflows, tooling, and best practices for contributing to reddit-cluster-map.

## Quick Start for New Developers

1. **Clone and setup**:
   ```bash
   git clone https://github.com/subculture-collective/clustr.git
   cd clustr/backend
   make setup
   ```
   This will create `.env` from `.env.example` and check for required tools.

2. **Configure environment**:
   Edit `backend/.env` and set:
   - `REDDIT_CLIENT_ID` and `REDDIT_CLIENT_SECRET` (from https://www.reddit.com/prefs/apps)
   - `POSTGRES_PASSWORD` (choose a strong password)

3. **Install Git hooks** (recommended):
   ```bash
   cd .. # back to repo root
   ./scripts/install-hooks.sh
   ```
   This installs pre-commit hooks that automatically check formatting and types.

4. **Start services**:
   ```bash
   cd backend
   docker compose up -d --build
   make migrate-up-local
   ```

5. **Seed some data** (optional):
   ```bash
   make seed
   ```

## Makefile Targets

Run `make help` from the `backend/` directory to see all available targets. Key ones:

### Setup and Tools

- `make setup` - Initial setup (creates .env, checks tools)
- `make check-env` - Verify .env is configured
- `make check-tools` - Check if required tools are installed
- `make install-tools` - Install sqlc and golang-migrate (requires Go)

### Development

- `make reset` - Reset database (stop, start, migrate)
- `make migrate-up-local` - Run migrations against localhost
- `make generate` / `make sqlc` - Regenerate Go code from SQL
- `make precalculate` - Run graph precalculation
- `make start-crawler` / `make stop-crawler` - Control crawler service

### Testing and Quality

- `make test` - Run all Go unit tests
- `make test-integration` - Run integration tests (requires TEST_DATABASE_URL)
- `make benchmark-graph` (from backend/) - Benchmark graph query performance
- `make lint` - Run Go linters (go vet and gofmt check)
- `make fmt` - Auto-format Go code with gofmt
- `make smoke-test` - Run smoke tests (basic API health checks)
- `make seed` - Seed database with sample subreddits

### Logs

- `make logs-api` - Follow API server logs
- `make logs-crawler` - Follow crawler logs
- `make logs-db` - Follow database logs
- `make logs-all` - Follow all container logs

### Backups

- `make backup-now` - Create a database backup
- `make backups-ls` - List backup files
- `make backups-download-latest` - Download latest backup to ./backups/

## Development Workflow

### Making Code Changes

1. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make changes and test frequently**:
   ```bash
   # After editing Go code
   make fmt        # Format code
   make lint       # Check for issues
   make test       # Run tests
   
   # After editing SQL queries
   make generate   # Regenerate sqlc code
   ```

3. **Test your changes**:
   ```bash
   # Rebuild and restart services
   docker compose up -d --build
   
   # Run smoke tests
   make smoke-test
   
   # Check logs
   make logs-api
   ```

4. **Commit** (pre-commit hook will run automatically):
   ```bash
   git add .
   git commit -m "Your commit message"
   ```
   
   The pre-commit hook will:
   - Check Go formatting (gofmt)
   - Run go vet
   - Run ESLint on TypeScript/JavaScript
   - Run TypeScript type checking

### Working with the Database

#### Migrations

All schema changes must be done via migrations:

1. Create new migration files in `backend/migrations/`:
   ```bash
   # Naming: NNNNNN_description.up.sql and NNNNNN_description.down.sql
   # where NNNNNN is next number (e.g., 000018)
   ```

2. Run migrations:
   ```bash
   make migrate-up-local  # for local development
   ```

3. Update `backend/migrations/schema.sql` to reflect the new state (this is used for fresh installs)

#### SQL Queries and sqlc

When adding or modifying database queries:

1. Edit SQL files in `backend/internal/queries/*.sql`
2. Regenerate Go code: `make generate`
3. The generated code appears in `backend/internal/db/`

Example query file (`backend/internal/queries/users.sql`):
```sql
-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY username LIMIT $1;
```

### Working with the Frontend

From the `frontend/` directory:

1. **Install dependencies**:
   ```bash
   npm ci
   ```

2. **Run development server**:
   ```bash
   npm run dev
   ```
   Access at http://localhost:5173 (proxies API to localhost:8000)

3. **Lint and type check**:
   ```bash
   npm run lint
   npx tsc --noEmit
   ```

4. **Build for production**:
   ```bash
   npm run build
   ```

### Testing

#### Unit Tests

Run all unit tests:
```bash
cd backend
make test
```

Test a specific package:
```bash
go test ./internal/crawler
```

Test with coverage:
```bash
go test -cover ./...
```

#### Integration Tests

Integration tests require a database. Set `TEST_DATABASE_URL`:
```bash
export TEST_DATABASE_URL="postgres://postgres:password@localhost:5432/reddit_cluster_test?sslmode=disable"
make test-integration
```

#### Performance Benchmarks

To measure graph query performance (from `backend/`):

```bash
# Ensure database is populated with graph data
make precalculate

# Run benchmarks
make benchmark-graph
```

The benchmark script tests query patterns used by the graph API and reports:
- Execution times (averaged over 5 runs)
- Index usage statistics
- Table statistics

For detailed performance analysis with EXPLAIN ANALYZE:

```bash
psql "$DATABASE_URL" -f backend/scripts/explain_analyze_queries.sql
```

See [Performance Documentation](perf.md) for interpreting results and optimization tips.

#### Smoke Tests

Smoke tests verify basic API functionality:
```bash
# Ensure services are running first
docker compose up -d

# Run smoke tests
make smoke-test

# Or with custom API URL
API_URL=https://your-domain.com make smoke-test
```

### Common Development Tasks

#### Adding a New API Endpoint

1. Add SQL query in `backend/internal/queries/` (if needed)
2. Run `make generate` to generate Go code
3. Add handler in `backend/internal/api/handlers/`
4. Register route in `backend/internal/api/routes.go`
5. Test with curl or smoke tests

#### Adding a New Crawler Feature

1. Edit `backend/internal/crawler/`
2. Add tests in `*_test.go` files
3. Test locally by triggering a crawl:
   ```bash
   make test-crawl SUB=golang
   make logs-crawler  # watch progress
   ```

#### Modifying the Graph Generation

1. Edit `backend/internal/graph/service.go`
2. Test with:
   ```bash
   make precalculate
   # Check logs for progress/errors
   # Verify graph endpoint
   curl http://localhost:8000/api/graph?max_nodes=10&max_links=10 | jq
   ```

## Pre-commit Hooks

Pre-commit hooks run automatically before each commit. They:

- **For Go files**:
  - Check formatting with gofmt
  - Run go vet for static analysis

- **For TypeScript/JavaScript files**:
  - Run ESLint
  - Run TypeScript type checking

To bypass hooks (not recommended):
```bash
git commit --no-verify
```

To reinstall hooks:
```bash
./scripts/install-hooks.sh
```

## Code Style

### Go

- Follow standard Go conventions
- Use `gofmt` for formatting (run `make fmt`)
- Pass `go vet` checks
- Write tests for new functionality
- Document exported functions and types

### TypeScript/React

- Use TypeScript for type safety
- Follow ESLint configuration
- Use functional components with hooks
- Keep components focused and composable

### SQL

- Use lowercase for SQL keywords in query files
- Use descriptive query names in sqlc comments
- Keep queries focused and efficient

## Environment Variables

### Backend (.env)

Required:
- `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET` - OAuth credentials
- `POSTGRES_PASSWORD` - Database password

Common development overrides:
- `DETAILED_GRAPH=true` - Include posts/comments in graph
- `LOG_HTTP_RETRIES=true` - Verbose HTTP retry logging
- `DISABLE_API_GRAPH_JOB=true` - Disable hourly graph job

See `backend/.env.example` for full list.

### Frontend (.env)

- `VITE_API_URL` - API endpoint (default: `/api`)
- Optional: `VITE_MAX_RENDER_NODES`, `VITE_MAX_RENDER_LINKS`

## Troubleshooting

### "make: *** No rule to make target '.env'"

Run `make setup` to create .env from .env.example.

### "sqlc not found" or "migrate not found"

Install tools:
```bash
make install-tools
# Or manually install from:
# https://docs.sqlc.dev/
# https://github.com/golang-migrate/migrate
```

### Database connection errors

1. Check services are running: `docker compose ps`
2. Check database logs: `make logs-db`
3. Verify .env has correct POSTGRES_PASSWORD
4. Try resetting: `make reset`

### Tests failing

1. Ensure database is running
2. Run migrations: `make migrate-up-local`
3. Check for unformatted code: `make lint`

### Crawler not working

1. Check OAuth credentials in .env
2. Check rate limiting (should be ~1.66 requests/sec)
3. Monitor logs: `make logs-crawler`
4. Verify jobs are enqueued: `curl http://localhost:8000/jobs`

## Additional Resources

- [API Documentation](./api.md)
- [Setup Guide](./setup.md)
- [Architecture Overview](./overview.md)
- [CI/CD Pipeline](./CI-CD.md)
- [Community Detection](./community-detection.md)
