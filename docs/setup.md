# Setup and Quickstart

This guide gets you from zero to a running stack with detailed explanations of the Docker Compose setup, environment configuration, and migration system.

## Prerequisites

- Docker and Docker Compose (Docker Engine 20.10+ recommended)
- Node.js 20+ (for local frontend builds)
- Go 1.21+ (optional for local backend builds)
- PostgreSQL client tools (optional, for direct database access)

## Clone and configure

1. Clone the repo:
   ```bash
   git clone https://github.com/subculture-collective/clustr.git
   cd clustr
   ```

2. **Quick setup** (recommended):
   ```bash
   cd backend
   make setup
   ```
   This creates `.env` from `.env.example` and checks for required tools.

3. Configure `backend/.env`:
   - Set `REDDIT_CLIENT_ID` and `REDDIT_CLIENT_SECRET` (from https://www.reddit.com/prefs/apps)
   - Set `POSTGRES_PASSWORD` to a strong password
   - Optionally adjust other settings (see comments in `.env.example`)

4. (Optional) Configure `frontend/.env`:
   ```bash
   cp frontend/.env.example frontend/.env
   ```
   For local dev, the defaults should work fine.

### Manual Configuration (Alternative)

If you prefer not to use `make setup`, manually copy the example files:

```bash
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
```

Then edit `backend/.env` with:

```bash
REDDIT_CLIENT_SECRET=<from reddit app>
REDDIT_REDIRECT_URI=<your callback, e.g. https://your.domain/oauth/reddit/callback>
REDDIT_SCOPES="identity read"
REDDIT_USER_AGENT=reddit-cluster-map/0.1 (+your_contact)

POSTGRES_PASSWORD=<choose strong password>
DATABASE_URL=postgres://postgres:${POSTGRES_PASSWORD}@db:5432/reddit_cluster?sslmode=disable

# Optional graph generation knobs
DETAILED_GRAPH=false
POSTS_PER_SUB_IN_GRAPH=10
COMMENTS_PER_POST_IN_GRAPH=50
MAX_AUTHOR_CONTENT_LINKS=3
DISABLE_API_GRAPH_JOB=false
PRECALC_CLEAR_ON_START=false

# Precalc batching & logs (runtime-tuned in Service)
GRAPH_NODE_BATCH_SIZE=1000
GRAPH_LINK_BATCH_SIZE=2000
GRAPH_PROGRESS_INTERVAL=10000
```

Then edit `frontend/.env` with:

```bash
VITE_API_URL=/api
```

## Required Tools

Before starting, ensure you have:

- Docker and Docker Compose
- Node.js 20+ (for local frontend builds)
- Go 1.21+ (optional for local backend builds)

Check installed tools:
```bash
cd backend
make check-tools
```

Install Go development tools (sqlc and golang-migrate):
```bash
make install-tools
```

## Docker Compose Architecture

The `backend/docker-compose.yml` file defines the following services:

### Core Services

1. **db** (PostgreSQL 17)
   - Base database with data checksums enabled
   - Volume: `postgres_data` for persistent storage
   - Initial schema loaded from `migrations/schema.sql`
   - Ports: Internal only (5432) within Docker network
   - Configuration: WAL enabled for replication support

2. **api** (Go API Server)
   - Exposes REST API on port 8000
   - Health check endpoint: `GET /health`
   - Depends on: `db`
   - Mounts: `pgbackups` volume (read-only) for backup file access via API
   - Auto-restarts on failure

3. **crawler** (Go Crawler Worker)
   - Processes crawl jobs from the database
   - Rate-limited to ~1.66 requests/second (601ms between requests)
   - Depends on: `db`
   - Runs continuously, polling for jobs

4. **precalculate** (Graph Generation)
   - Runs graph precalculation hourly
   - Executes database backups hourly (via mounted scripts)
   - Depends on: `db`
   - Mounts: `./scripts` directory and `pgbackups` volume

5. **backup** (Scheduled Backups)
   - Standalone backup service with configurable interval
   - Environment: `BACKUP_INTERVAL` (default: 24h)
   - Writes to `pgbackups` volume

6. **reddit_frontend** (React/Vite Frontend)
   - Serves the web UI with nginx
   - Port: 80 (container)
   - Proxies `/api/*` requests to the API service
   - Depends on: `api`

### Monitoring Services

7. **prometheus** (Metrics Collection)
   - Scrapes metrics from API server (`/metrics` endpoint)
   - Port: 9090 (exposed to host)
   - Volume: `prometheus_data` for time-series storage
   - Configuration: `monitoring/prometheus/prometheus.yml`

8. **grafana** (Dashboards and Visualization)
   - Visualizes Prometheus metrics
   - Port: 3000 (exposed to host)
   - Default credentials: admin / ${GRAFANA_ADMIN_PASSWORD}
   - Volume: `grafana_data` for dashboard persistence
   - Pre-configured dashboards in `monitoring/grafana/provisioning/`

### Named Volumes

- `postgres_data` - PostgreSQL data directory
- `pgbackups` - Database backup files
- `prometheus_data` - Prometheus time-series data
- `grafana_data` - Grafana configuration and dashboards

### Networking

All services communicate via the external `web` network. This allows:
- Service-to-service communication using service names as hostnames
- Optional integration with reverse proxies (nginx, Traefik, etc.)

## Start services

From `backend/`:

1. **Create the external network** (first time only):
   ```bash
   docker network create web
   ```

2. **Start all services**:
   ```bash
   docker compose up -d --build
   ```
   
   This will:
   - Build custom Docker images for api, crawler, precalculate, and frontend
   - Start all services in detached mode
   - Initialize the database with the base schema
   - Begin health checks on the API service

3. **Run migrations** to ensure all schema changes are applied:
   ```bash
   make migrate-up-local
   ```
   
   Or if using remote database:
   ```bash
   make migrate-up  # Auto-detects DATABASE_URL from environment
   ```
   
   **Important migrations:**
   - Migration 000016 adds position columns (`pos_x`, `pos_y`, `pos_z`) to `graph_nodes` for layout computation
   - Migrations are idempotent and safe to run multiple times

4. **Verify services are running**:
   ```bash
   docker compose ps
   ```
   
   All services should show "running" status. Check health of API:
   ```bash
   curl http://localhost:8000/health
   ```

5. **Check position columns** (verify migration success):
   ```bash
   make logs-api | grep "Position columns"
   ```
   
   Expected output:
   - ✓ Success: "Position columns (pos_x, pos_y, pos_z) are present in graph_nodes table"
   - ⚠️ Warning: "Position columns missing..." indicates migrations need to run

### Service Endpoints

- **API Server**: http://localhost:8000
- **Frontend**: http://localhost (port 80) or via your reverse proxy
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000
- **Database**: localhost:5432 (if exposed, otherwise internal only)

## Seed a crawl

### Quick Seed (Automated)

The fastest way to populate the database:

```bash
cd backend
make seed
```

This will enqueue crawl jobs for several popular subreddits (AskReddit, programming, golang, webdev, dataisbeautiful).

### Manual Seed

Alternatively, POST to enqueue individual subreddits:

```
curl -X POST https://<your-domain>/api/crawl -H 'Content-Type: application/json' -d '{"subreddit":"AskReddit"}'
```

Crawler will pull posts/comments and populate the DB.

Check job status:
```bash
curl http://localhost:8000/jobs | jq
# Or for production:
curl https://<your-domain>/jobs | jq
```

Monitor crawler progress:
```bash
make logs-crawler
```

## Generate graph now

- Run the precalculate container once:
  - `docker compose run --rm precalculate /app/precalculate`

Or wait for the API server’s graph job (every hour) to update automatically.

## Frontend access

- Open your domain. The app fetches `/api/graph` via nginx.
- To include precomputed 3D positions in the API response, use: `/api/graph?with_positions=true`
  - Positions are only returned when the `pos_x`, `pos_y`, `pos_z` columns are present and populated

## Local dev (optional)

### Smoke Tests

Verify all API endpoints are working:
```bash
cd backend
make smoke-test
```

This checks connectivity and basic functionality of all API endpoints.

### Backend Development

- Run tests:
  ```bash
  make test
  ```

- Check code formatting and quality:
  ```bash
  make lint
  ```

- Format code:
  ```bash
  make fmt
  ```

- Regenerate sqlc after changing SQL:
  ```bash
  make generate
  ```

Run `make help` for all available targets.

### Frontend Development

From `frontend/`:

- Install dependencies: `npm ci`
- Start dev server: `npm run dev` (proxy to `localhost:8000` is configured in `vite.config.ts`)
- Lint: `npm run lint`
- Type check: `npx tsc --noEmit`
- Build: `npm run build`

### Git Hooks (Recommended)

Install pre-commit hooks for automatic formatting and type checking:
```bash
./scripts/install-hooks.sh
```

This will run checks before each commit to ensure code quality.

See the **[Developer Guide](./developer-guide.md)** for comprehensive development workflows and best practices.

## Complete Environment Variables Reference

The backend uses environment variables for all configuration. Below is a complete reference organized by category.

### Reddit OAuth (Required)

```bash
REDDIT_CLIENT_ID=your_client_id_here              # From https://www.reddit.com/prefs/apps
REDDIT_CLIENT_SECRET=your_client_secret_here      # From Reddit app settings
REDDIT_REDIRECT_URI=http://localhost:8000/oauth/reddit/callback
REDDIT_SCOPES="identity read"                     # OAuth scopes needed
REDDIT_USER_AGENT=reddit-cluster-map/0.1 (+your@email.com)
```

### Database Configuration (Required)

```bash
POSTGRES_USER=postgres                            # Database username
POSTGRES_PASSWORD=change_me_in_production         # Strong password required
POSTGRES_DB=reddit_cluster                        # Database name
DATABASE_URL=postgres://postgres:${POSTGRES_PASSWORD}@db:5432/reddit_cluster?sslmode=disable
```

### Security & Rate Limiting

```bash
ENABLE_RATE_LIMIT=true                            # Enable/disable rate limiting
RATE_LIMIT_GLOBAL=100                             # Global requests per second
RATE_LIMIT_GLOBAL_BURST=200                       # Global burst capacity
RATE_LIMIT_PER_IP=10                              # Per-IP requests per second
RATE_LIMIT_PER_IP_BURST=20                        # Per-IP burst capacity
CORS_ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000  # CORS allowed origins
ADMIN_API_TOKEN=                                  # Bearer token for admin endpoints
```

### HTTP Client & Retries

```bash
HTTP_MAX_RETRIES=3                                # Maximum retry attempts
HTTP_RETRY_BASE_MS=300                            # Base retry delay in milliseconds
HTTP_TIMEOUT_MS=15000                             # HTTP request timeout
LOG_HTTP_RETRIES=false                            # Log retry attempts
GRAPH_QUERY_TIMEOUT_MS=30000                      # Graph API query timeout
DB_STATEMENT_TIMEOUT_MS=25000                     # Database statement timeout
```

### Graph Generation

```bash
DETAILED_GRAPH=false                              # Include posts/comments in graph
POSTS_PER_SUB_IN_GRAPH=10                        # Max posts per subreddit
COMMENTS_PER_POST_IN_GRAPH=50                    # Max comments per post
MAX_AUTHOR_CONTENT_LINKS=3                       # Cross-link author content
DISABLE_API_GRAPH_JOB=false                      # Disable hourly background job
PRECALC_CLEAR_ON_START=false                     # Clear graph tables before precalc
```

### Graph Precalculation Performance

```bash
GRAPH_NODE_BATCH_SIZE=1000                       # Nodes per batch insert
GRAPH_LINK_BATCH_SIZE=2000                       # Links per batch insert
GRAPH_PROGRESS_INTERVAL=10000                    # Progress log interval
```

### Crawler Configuration

```bash
STALE_DAYS=30                                     # Days before subreddit is stale
RESET_CRAWLING_AFTER_MIN=15                      # Reset stuck jobs after minutes
```

### Monitoring

```bash
GRAFANA_ADMIN_PASSWORD=admin                      # Grafana admin password
```

### Testing & Development

```bash
TEST_DATABASE_URL=postgres://testuser:testpass@localhost:5433/testdb?sslmode=disable
```

## Migration System

The project uses [golang-migrate](https://github.com/golang-migrate/migrate) for database schema versioning.

### Migration Files

Migrations are stored in `backend/migrations/`:

- **Numbered pairs**: Each migration has an "up" and "down" file
  - `000001_initial.up.sql` - Apply changes
  - `000001_initial.down.sql` - Revert changes
  
- **schema.sql**: Base schema for fresh installations (used by Docker init)

### Running Migrations

**Local development** (database at localhost:5432):
```bash
cd backend
make migrate-up-local
```

**Production or custom DATABASE_URL**:
```bash
export DATABASE_URL="postgres://user:pass@host:5432/dbname?sslmode=disable"
make migrate-up
```

**Docker exec** (direct SQL execution, not recommended):
```bash
make migrate  # Runs schema.sql only, doesn't track version
```

### Migration Best Practices

1. **Always create both up and down migrations**
2. **Test migrations** before committing:
   ```bash
   # Apply migration
   make migrate-up-local
   
   # Verify schema changes
   psql $DATABASE_URL -c "\d table_name"
   
   # Test rollback (if safe)
   migrate -path migrations -database "$DATABASE_URL" down 1
   ```

3. **Keep migrations idempotent** where possible:
   ```sql
   -- Example: Add column only if it doesn't exist
   ALTER TABLE graph_nodes 
   ADD COLUMN IF NOT EXISTS pos_x DOUBLE PRECISION;
   ```

4. **Update schema.sql** after creating migrations:
   - Apply your migration to a fresh database
   - Export the schema: `pg_dump -s > migrations/schema.sql`

### Migration Status

Check current migration version:
```bash
migrate -path backend/migrations -database "$DATABASE_URL" version
```

### Troubleshooting Migrations

**"Dirty database version"** error:
```bash
# Force set version (use carefully!)
migrate -path backend/migrations -database "$DATABASE_URL" force VERSION_NUMBER
```

**Migration fails mid-way**:
1. Check error in logs
2. Fix the issue (manual SQL or code correction)
3. Force to the version you want
4. Continue migrations

## Troubleshooting

### Service Issues

**Services not starting:**
```bash
# Check service status
docker compose ps

# Check logs for specific service
make logs-api
make logs-crawler
make logs-db

# Restart specific service
docker compose restart api

# Full reset
make reset
```

**Database connection errors:**
1. Verify database is running: `docker compose ps db`
2. Check credentials in `.env` match what's expected
3. Check logs: `make logs-db`
4. Try connecting directly:
   ```bash
   docker compose exec db psql -U postgres -d reddit_cluster
   ```

**Health check failing:**
```bash
# Check API health endpoint
curl http://localhost:8000/health

# View detailed logs
make logs-api

# Common causes:
# - Database connection failed (check DATABASE_URL)
# - Missing migrations (run make migrate-up-local)
# - Port already in use (stop conflicting service)
```

### Crawler Issues

**403 on user listings**: 
- App-only OAuth may be blocked by Reddit
- Code automatically falls back to search or public endpoints
- This is expected behavior, not an error

**Rate limit errors**:
- All requests are globally paced at 601ms (≈1.66 rps)
- This is intentional to respect Reddit's rate limits
- To adjust, edit `internal/crawler/ratelimit.go`

**Crawler not processing jobs:**
```bash
# Check crawler logs
make logs-crawler

# Verify jobs exist
curl http://localhost:8000/jobs | jq

# Manually enqueue a job
make test-crawl SUB=golang

# Restart crawler
docker compose restart crawler
```

### Frontend Issues

**Double /api in URLs**:
- Ensure `VITE_API_URL` has no trailing slash in `frontend/.env`
- Should be: `VITE_API_URL=/api` (not `/api/`)

**Blank graph display**:
1. Ensure precalc has run: `docker compose logs precalculate`
2. Check API returns data: `curl http://localhost:8000/api/graph?max_nodes=10 | jq`
3. Verify `DETAILED_GRAPH` and related settings
4. Run precalc manually: `docker compose run --rm precalculate /app/precalculate`

### Graph Issues

**Empty graph (/api/graph returns no nodes)**:
- Precalculation hasn't run yet (wait for hourly job or run manually)
- No data in database (seed some data with `make seed`)
- Tables are empty: Check with `docker compose exec db psql -U postgres -d reddit_cluster -c "SELECT COUNT(*) FROM graph_nodes;"`

**Precalculation slow**:
- Adjust batch sizes: `GRAPH_NODE_BATCH_SIZE`, `GRAPH_LINK_BATCH_SIZE`
- Reduce data volume: Lower `POSTS_PER_SUB_IN_GRAPH` or `COMMENTS_PER_POST_IN_GRAPH`
- Monitor progress: `docker compose logs -f precalculate`

**Position columns errors** (`pos_x/pos_y/pos_z does not exist`):
- Run migrations: `make migrate-up-local`
- Migration 000016 adds these columns
- Safe to run on existing databases (idempotent)

### Performance Issues

**Slow API responses**:
1. Check database performance: `make logs-db`
2. Run benchmarks: `make benchmark-graph`
3. Check if indexes exist (should be created by migrations)
4. Consider increasing `DB_STATEMENT_TIMEOUT_MS`

**High memory usage**:
- Reduce batch sizes in precalculation
- Limit graph size with `max_nodes` and `max_links` query params
- Check for memory leaks in logs

For more operational troubleshooting, see [docs/runbooks.md](./runbooks.md).
