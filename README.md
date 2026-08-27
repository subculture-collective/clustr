# Reddit Cluster Map

[![CI](https://github.com/onnwee/reddit-cluster-map/actions/workflows/ci.yml/badge.svg)](https://github.com/onnwee/reddit-cluster-map/actions/workflows/ci.yml)
[![Version](https://img.shields.io/badge/version-0.1.0-blue.svg)](VERSION)

Collect, analyze, and visualize relationships between Reddit communities and users as an interactive 3D network graph.

**[Changelog](CHANGELOG.md)** • **[Contributing](CONTRIBUTING.md)** • **[Releases](https://github.com/subculture-collective/reddit-cluster-map/releases)**

---

## 🧠 What it does

- Crawls subreddits for posts and comments (OAuth-authenticated; globally rate limited).
- Stores normalized data in PostgreSQL.
- Precomputes a graph (nodes + links) based on shared participation and activity, with an optional detailed content graph (posts/comments).
- Serves the graph at `/api/graph` for the React frontend to render in multiple visualization modes:
  - **3D Graph**: Interactive WebGL visualization
  - **2D Graph**: SVG-based force-directed layout with drag & pan
  - **Dashboard**: Statistical overview and analytics
  - **Communities**: Automated community detection using the Louvain algorithm

---

## 🧱 Architecture

- Backend (Go)
  - API server: `backend/cmd/server`
  - Crawler: `backend/cmd/crawler`
  - Precalculation: `backend/cmd/precalculate`
  - Data access via sqlc: SQL in `backend/internal/queries/*.sql` → generated in `backend/internal/db`
- Database: PostgreSQL
- Frontend (Vite + React 3D): `frontend/` (graph viewer)
- Monitoring: Prometheus + Grafana for metrics and dashboards

See `docs/overview.md` for the full system picture and data flow.

---

## 🚀 Quick start

For full setup (Docker, env vars, seeding a crawl), see `docs/setup.md`.
For CI/CD pipeline and Docker image publishing, see `docs/CI-CD.md`.

Common dev tasks from `backend/`:

- Setup environment file:
  - `make setup` (creates `.env` from `.env.example`)
- Regenerate sqlc after editing SQL in `backend/internal/queries/*.sql`:
  - `make sqlc` (alias: `make generate`)
- Run the one-shot graph precalc:
  - `make precalculate`
- Run tests:
  - `go test ./...`
### For New Developers

1. Clone and setup:
   ```bash
   git clone https://github.com/subculture-collective/reddit-cluster-map.git
   cd reddit-cluster-map/backend
   make setup  # Creates .env and checks tools
   ```

2. Configure `backend/.env` with your Reddit OAuth credentials and database password

3. Start services:
   ```bash
   docker compose up -d --build
   make migrate-up-local
   ```

4. (Optional) Seed sample data and run smoke tests:
   ```bash
   make seed
   make smoke-test
   ```

See the **[Developer Guide](docs/developer-guide.md)** for detailed workflows, testing, and best practices.

### Documentation

#### Getting Started

- **[Setup Guide](docs/setup.md)** - Complete setup with Docker Compose, migrations, environment variables
- **[Developer Guide](docs/developer-guide.md)** - Development workflows, Makefile targets, testing, and troubleshooting
- **[Contributing Guide](CONTRIBUTING.md)** - How to contribute, coding standards, PR guidelines

#### Architecture & Design

- **[Architecture Overview](docs/architecture.md)** - System architecture with diagrams, data flow, component interactions
- **[Overview](docs/overview.md)** - High-level system design and data flow

#### Operations

- **[Runbooks](docs/runbooks.md)** - Operational procedures: backup/restore, maintenance, troubleshooting
- **[Production Database Clone](docs/runbooks/production-database-clone.md)** - Snapshot-consistent encrypted Almaz-to-Kvant export, restore, verification, and cleanup
- **[Almaz + Kvant Launch](docs/runbooks/almaz-kvant-launch.md)** - Clone-qualified calculation and rollback-safe launch procedure
- **[Affinity World + Catalog v2](docs/runbooks/affinity-catalog-v2.md)** - Shadow build, coordinated promotion, monitoring, sitemap tiers, and atomic rollback
- **[Monitoring Guide](docs/monitoring.md)** - Metrics, Prometheus, Grafana dashboards, and alerts
- **[Data Integrity Guide](backend/docs/DATA_INTEGRITY.md)** - Database integrity checks and maintenance

#### Features & APIs

- **[API Documentation](docs/api.md)** - Core API endpoints and usage
- **[Community API](docs/api-communities.md)** - Community aggregation endpoints (supernodes and subgraphs)
- **[Community Detection](docs/community-detection.md)** - Louvain algorithm implementation
- **[Visualization Modes](docs/visualization-modes.md)** - 3D, 2D, dashboard, and community views

#### Advanced Topics

- **[Performance Documentation](docs/perf.md)** - Query optimization, benchmarking, and scaling
- **[Performance Profiling Guide](docs/profiling.md)** - Runtime profiling, benchmarks, and optimization
- **[Performance Analysis](docs/performance-analysis.md)** - Performance review and optimization recommendations
- **[Load Testing Guide](docs/load-testing.md)** - k6-based load testing for API performance validation
- **[OAuth Token Management](docs/oauth-token-management.md)** - Token refresh, credential rotation
- **[Crawler Resilience](docs/CRAWLER_RESILIENCE.md)** - Rate limiting, retries, circuit breakers
- **[Security Guide](docs/SECURITY.md)** - Security features and best practices
- **[Security Audit Summary](docs/SECURITY_AUDIT_SUMMARY.md)** - Quick reference for security testing and auditing
- **[Security Audit Guide](docs/SECURITY_AUDIT.md)** - Comprehensive security auditing and penetration testing procedures
- **[Penetration Testing Checklist](docs/PENETRATION_TESTING_CHECKLIST.md)** - Detailed penetration testing checklist
- **[CI/CD Pipeline](docs/CI-CD.md)** - Continuous integration and deployment

### Common Development Tasks

From `backend/`, run `make help` to see all available targets. Key ones:

- `make generate` - Regenerate sqlc code after editing SQL
- `make precalculate` - Run graph precalculation
- `make test` - Run all tests
- `make benchmark` - Run Go benchmark tests
- `make benchmark-graph` - Benchmark graph query performance
- `make performance-baseline` - Collect comprehensive performance baseline
- `make profile-cpu` - Collect CPU profile (requires ENABLE_PROFILING=true)
- `make profile-memory` - Collect memory profile (requires ENABLE_PROFILING=true)
- `make loadtest` - Run k6 load tests (smoke, load, stress, soak)
- `make loadtest-smoke` - Quick smoke test (30s)
- `make loadtest-load` - Load test with 50 VUs (5min)
- `make profile-all` - Collect all profiles (CPU, memory, goroutines)
- `make integrity-check` - Run data integrity checks
- `make integrity-clean` - Clean up data integrity issues
- `make lint` - Check code formatting and run go vet
- `make fmt` - Auto-format Go code
- `make smoke-test` - Run API health checks
- `make seed` - Populate database with sample data

---

## 🔌 API surface

- `GET /api/graph?max_nodes=20000&max_links=50000`
  - Returns `{ nodes, links }`. Results are cached for ~60s and capped by max_nodes/max_links using a stable weighting.
  - Prefers precalculated tables, falls back to legacy JSON when empty.
- `GET /api/communities?max_nodes=100&max_links=500&with_positions=true`
  - Returns aggregated community supernodes and inter-community weighted links.
  - Communities detected via server-side Louvain algorithm during precalculation.
- `GET /api/communities/{id}?max_nodes=10000&max_links=50000`
  - Returns the full subgraph (all nodes and links) for a specific community.
- `POST /api/crawl { "subreddit": "AskReddit" }`
- Additional resource endpoints exist without `/api` prefix: `/subreddits`, `/users`, `/posts`, `/comments`, `/jobs`.

See `docs/api.md` and `docs/api-communities.md` for details.

---

## 📊 Monitoring and Analytics

The project includes comprehensive monitoring with Prometheus and Grafana:

- **Metrics endpoint**: `GET /metrics` - Prometheus format metrics
- **Prometheus**: http://localhost:9090 - Metrics collection and querying
- **Grafana**: http://localhost:3000 - Dashboards and visualizations (default: admin/admin)

### Key Metrics

- **Crawl metrics**: Job throughput, success/failure rates, posts/comments processed
- **API metrics**: Request rates, response times (p50/p95/p99), error rates
- **Graph metrics**: Node/link counts by type, precalculation duration
- **Database metrics**: Operation durations, error rates
- **System health**: Circuit breaker status, rate limiting pressure

### Alerts

Pre-configured alerts for:
- High API error rates (>5%)
- High crawler error rates (>10%)
- Slow queries (p95 > 2s)
- Database errors
- Circuit breaker trips
- Stalled crawl jobs

See **[Monitoring Guide](docs/monitoring.md)** for complete metrics reference, dashboard setup, and PromQL examples.

---

## ⚙️ Configuration

Key environment variables (selected):

- **Security** (see `docs/SECURITY.md` for details)
  - `ENABLE_RATE_LIMIT` (true) — enable/disable rate limiting
  - `RATE_LIMIT_GLOBAL` (100) — global requests per second
  - `RATE_LIMIT_GLOBAL_BURST` (200) — global burst size
  - `RATE_LIMIT_PER_IP` (10) — requests per second per IP
  - `RATE_LIMIT_PER_IP_BURST` (20) — per-IP burst size
  - `CORS_ALLOWED_ORIGINS` — comma-separated list of allowed CORS origins (default: localhost:5173,localhost:3000)
  - `ADMIN_API_TOKEN` — bearer token for admin endpoints
- **Monitoring**
  - `GRAFANA_ADMIN_PASSWORD` (admin) — Grafana admin password
- Reddit OAuth
  - `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET`, `REDDIT_REDIRECT_URI`, `REDDIT_SCOPES`, `REDDIT_USER_AGENT`
- HTTP / retries
  - `HTTP_MAX_RETRIES` (default 3), `HTTP_RETRY_BASE_MS` (300), `HTTP_TIMEOUT_MS` (15000), `LOG_HTTP_RETRIES` (false)
  - `GRAPH_QUERY_TIMEOUT_MS` (30000) — timeout for graph API queries
  - `DB_STATEMENT_TIMEOUT_MS` (25000) — database statement timeout
- Graph generation
  - `DETAILED_GRAPH` (false) — include posts/comments
  - `POSTS_PER_SUB_IN_GRAPH` (10), `COMMENTS_PER_POST_IN_GRAPH` (50)
  - `MAX_AUTHOR_CONTENT_LINKS` (3) — cross-link content by the same author across subreddits
  - `DISABLE_API_GRAPH_JOB` (false) — disable hourly background job in API
  - `PRECALC_CLEAR_ON_START` (false) — when true, clears graph tables at precalc start
  - Batching/progress (applied at runtime in precalc):
    - `GRAPH_NODE_BATCH_SIZE` (1000)
    - `GRAPH_LINK_BATCH_SIZE` (2000)
    - `GRAPH_PROGRESS_INTERVAL` (10000)
- Crawler scheduling
  - `STALE_DAYS` (30), `RESET_CRAWLING_AFTER_MIN` (15)

---

## 🖥 Frontend

- Vite + React with multiple visualization modes:
  - 3D graph with `react-force-graph-3d` and interactive minimap for navigation
  - 2D graph with D3.js force simulation
  - Statistics dashboard
  - **Community detection** view with Louvain algorithm
- `VITE_API_URL` defaults to `/api`
- Optional client caps: `VITE_MAX_RENDER_NODES`, `VITE_MAX_RENDER_LINKS`

See `docs/visualization-modes.md` and `docs/community-detection.md` for feature details.
See `frontend/README.md` for local dev and env hints.

## License

Licensed under `GPL-3.0-or-later`. See [LICENSE](LICENSE).
