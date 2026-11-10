# Clustr

![Clustr identity](docs/brand/clustr/readme-lockup.png)

Clustr maps Reddit communities by the people they share. It crawls subreddits,
links them through common users and activity, and lets you fly through the
result as a 3D network, or read it as a 2D graph, a dashboard or a searchable
catalog.

It is live at [clustr.subcult.tv](https://clustr.subcult.tv). The version in
[`VERSION`](VERSION) is 0.1.0, and the project is in active development.

## Features

- **Crawler:** fetches posts and comments from subreddits through Reddit
  OAuth, with global rate limiting, retries and a circuit breaker. Crawl
  requests are leased and retried, and new subreddits are discovered from
  mentions and author history within daily budgets.
- **Graph builds:** a precalculation job turns crawled data into a weighted
  graph of subreddits, users, posts and comments, detects communities with the
  Louvain algorithm, and publishes each build as an immutable revision.
- **Spatial catalog:** every entity gets a stable 3D position, with subreddits
  placed by a force simulation over shared users.
- **Explorer:** a WebGL 3D view with level-of-detail, labels, community
  landmarks and a minimap; a 2D force-directed view; a statistics dashboard;
  and a communities view.
- **Search and sharing:** search by name, open any entity by link, and share
  the current camera and filters through the URL. Keyboard shortcuts are listed
  in [frontend/KEYBOARD_SHORTCUTS.md](frontend/KEYBOARD_SHORTCUTS.md)
  ([help overlay](keyboard-shortcuts-help-overlay.png)).
- **Catalog view:** a paged, filterable list of every entity in a revision,
  behind the `VITE_CATALOG_ENABLED` and `CATALOG_ENABLED` flags.
- **API:** graph, region, community, entity, search, diff, catalog and sitemap
  endpoints. See [docs/api.md](docs/api.md),
  [docs/api-communities.md](docs/api-communities.md) and
  [docs/api-catalog-v2.md](docs/api-catalog-v2.md).
- **Admin and monitoring:** token-protected admin endpoints for crawl control,
  cache and backups; Prometheus metrics at `/metrics`, with Grafana dashboards
  and alert rules in `monitoring/`.

Clustr Galaxy, a separate Unity client, reads the same API.

## Quick start

You need Go (version in `backend/go.mod`), Node.js, Docker, and
[sqlc](https://sqlc.dev/) and
[golang-migrate](https://github.com/golang-migrate/migrate) (`make
install-tools` installs both).

```bash
make setup                 # creates backend/.env from backend/.env.example
# edit backend/.env: Reddit OAuth credentials, database password, ADMIN_API_TOKEN,
# and DATABASE_URL pointing at localhost:5432 instead of db:5432
docker network create web  # the Compose stack uses an external network named web
docker compose -f backend/docker-compose.yml up -d db
make migrate-up-host       # applies backend/migrations to localhost:5432
make dev-backend           # API on port 8000 (API_PORT)
make dev-frontend          # Vite dev server; proxies /api to localhost:8000
```

Queue a crawl, then build the graph. The crawler and precalculation read only
the environment, so these commands load `backend/.env` with the `godotenv` tool
from the backend's Go module:

```bash
cd backend
alias withenv='go run github.com/joho/godotenv/cmd/godotenv -f .env'
withenv sh -c 'curl -X POST http://localhost:8000/api/crawl \
  -H "Authorization: Bearer $ADMIN_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"subreddit\": \"AskReddit\"}"'
withenv go run ./cmd/crawler        # keep running in another terminal
withenv go run ./cmd/precalculate   # one-shot graph build
```

Tests and checks:

```bash
make test        # backend go test ./... and frontend vitest
make lint        # go vet, gofmt check and eslint
make generate    # regenerate sqlc code after editing backend/internal/queries/*.sql
```

`make help` lists every target, including load tests (`make loadtest-*`),
integrity checks and profiling. The [setup guide](docs/setup.md) and
[developer guide](docs/developer-guide.md) go into more detail.

## Configuration

The backend reads `backend/.env`; `backend/.env.example` lists every variable
with comments. The main groups:

- **Reddit:** `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET`,
  `REDDIT_REDIRECT_URI`, `REDDIT_SCOPES`, `REDDIT_USER_AGENT`
- **Database:** `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`,
  `DATABASE_URL`, `DB_STATEMENT_TIMEOUT_MS`
- **API and security:** `API_PORT`, `ADMIN_API_TOKEN`, `CORS_ALLOWED_ORIGINS`,
  `ENABLE_RATE_LIMIT`, `RATE_LIMIT_*`, `PUBLIC_BASE_URL`
- **Crawler:** `CRAWLER_RPS`, `CRAWLER_BURST_SIZE`, `STALE_DAYS`, `CRAWL_*`,
  `DISCOVERY_*`, `HTTP_*`
- **Graph builds:** `DETAILED_GRAPH`, `POSTS_PER_SUB_IN_GRAPH`,
  `COMMENTS_PER_POST_IN_GRAPH`, `GRAPH_*`, `LAYOUT_*`, `PUBLICATION_*`,
  `PRECALC_*`
- **Catalog and sitemaps:** `CATALOG_ENABLED`, `CATALOG_CURSOR_SECRET`,
  `SITEMAP_TIER*`
- **Observability:** `LOG_LEVEL`, `OTEL_*`, `SENTRY_*`, `ENABLE_PROFILING`
- **Cache:** `CACHE_MAX_SIZE_MB`, `CACHE_MAX_ENTRIES`, `CACHE_TTL_SECONDS`

The frontend reads `VITE_API_URL` (default `/api`), `VITE_PROXY_TARGET` for the
dev server, `VITE_CATALOG_ENABLED`, and optional render caps
`VITE_MAX_RENDER_NODES` and `VITE_MAX_RENDER_LINKS`. See
`frontend/.env.example`.

## Deploy

`backend/docker-compose.yml` runs the whole stack: PostgreSQL, a one-shot
migration job, the API, the crawler, the precalculation job, a backup job, the
nginx frontend, Prometheus and Grafana. Services join an external Docker
network named `web`, so put a reverse proxy on that network in front of the
frontend container, which serves the app and proxies `/api` to the API.

```bash
docker network create web
docker compose -f backend/docker-compose.yml up -d --build
```

The images are built from `backend/Dockerfile` (API),
`backend/Dockerfile.crawler`, `backend/Dockerfile.precalculate`, `backend/Dockerfile.migrate`,
`backend/Dockerfile.integrity` and `frontend/Dockerfile`. Run the
precalculation job on a schedule so the graph stays current. Backups, restores
and monitoring are covered in [docs/runbooks.md](docs/runbooks.md) and
[docs/monitoring.md](docs/monitoring.md).

## Project layout

```text
backend/cmd/        server, crawler, precalculate, migrate, integrity and catalog tools
backend/internal/   API, crawler, graph builds, queries (sqlc) and storage
backend/migrations/ PostgreSQL migrations
frontend/           React, TypeScript and Three.js explorer
monitoring/         Prometheus and Grafana configuration
docs/               Architecture, API and operations documentation
```

[docs/architecture.md](docs/architecture.md) describes the data flow.

## Contributing

- Report bugs and request features in
  [GitHub issues](https://github.com/subculture-collective/clustr/issues).
- Ask questions and share ideas in
  [GitHub Discussions](https://github.com/subculture-collective/clustr/discussions).
- Send code as a pull request on GitHub against `main`.

CI runs backend tests against a migrated database with the race detector,
`go vet` and security scans, frontend lint and tests, visual regression tests,
performance benchmarks, Docker image builds and secret scanning on every pull
request. The `Mirror CI gate` check must pass before a pull request can merge.
See [CONTRIBUTING.md](CONTRIBUTING.md) for coding standards and the
[changelog](CHANGELOG.md) for release notes.

## License

Clustr is licensed under GPL-3.0-or-later. See [LICENSE](LICENSE). Built by
[SUBCULT](https://subcult.tv).
