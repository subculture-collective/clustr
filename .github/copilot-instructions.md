# Copilot Instructions — Reddit Cluster Map

> These instructions apply to **all Copilot agents** (Chat, Edits, Coding Agent) working
> in this repository. Keep them in sync with the codebase; if you change an architectural
> decision, update these instructions in the same PR.

---

## 1 · Project overview

Reddit Cluster Map crawls Reddit, stores normalized data in PostgreSQL, precomputes a
relationship graph, and serves it to an interactive React frontend.

**Current status:** v0.1.0 — MVP feature-complete (Milestones M1–M4 done).
**Active roadmap:** Issue **#138** — *"MVP to Professional Grade"* v2.0, targeting 100k+
node rendering, streaming APIs, GPU-instanced rendering, and production-grade ops.

### Repository layout

```
Makefile                         # Root — unified dev/deploy commands
backend/
  cmd/
    server/main.go               # API server entry
    crawler/main.go              # Crawler entry
    precalculate/main.go         # Graph precalc entry
    integrity/main.go            # Data integrity checks
  internal/
    api/                         # HTTP handlers, routes, middleware
      handlers/                  # Route handlers (graph.go, crawl.go, etc.)
      routes.go                  # Route registration
    config/                      # Env-based config (config.Load(), memoized)
    crawler/                     # Reddit crawl logic + rate limiter
    db/                          # sqlc-generated Go code (DO NOT EDIT)
    graph/                       # Precalculation service
    httpx/                       # HTTP client with retries + backoff
    metrics/                     # Prometheus metric definitions
    middleware/                  # Auth, CORS, rate-limit, logging
    queries/                     # *.sql source files for sqlc
    scheduler/                   # Crawl job scheduling
    ...                          # authstore, circuitbreaker, integrity,
                                 # logger, redditapi, secrets, tracing, utils
  migrations/                    # golang-migrate SQL files
  docker-compose.yml             # Full stack (db, api, crawler, precalc,
                                 #   frontend, prometheus, grafana, backups)
frontend/
  src/
    components/
      Graph3D.tsx                # 3D visualization (react-force-graph-3d)
      Graph2D.tsx                # 2D force layout (D3)
      CommunityMap.tsx           # Community detection view
      Dashboard.tsx              # Statistics dashboard
      Admin.tsx                  # Admin control panel
      Inspector.tsx              # Node inspector
      Controls.tsx, Legend.tsx, ShareButton.tsx, VirtualList.tsx
    types/graph.ts               # Shared type definitions
    utils/                       # Frontend utilities
  e2e/                           # Playwright end-to-end tests
monitoring/
  prometheus/                    # prometheus.yml, alert rules
  grafana/                       # Dashboard provisioning
scripts/                         # deploy.sh, install-hooks.sh, etc.
docs/                            # Architecture, API docs, runbooks, security
```

---

## 2 · Architecture & data flow

### Crawl → Store → Precalculate → Serve → Render

1. **Crawl** — `POST /api/crawl {"subreddit":"…"}` enqueues a job. The crawler
   processes it via OAuth-authenticated, rate-limited HTTP (~1.66 rps global ticker in
   `internal/crawler/ratelimit.go`). Retries respect `Retry-After` + exponential backoff
   (`internal/httpx/httpx.go`).

2. **Store** — Normalized tables: `subreddits`, `users`, `posts`, `comments`,
   `crawl_jobs`. All DB access goes through **sqlc**-generated code.

3. **Precalculate** — Runs hourly (also on-demand via `make precalculate`). Writes to
   `graph_nodes` and `graph_links`. Community detection uses multi-level Louvain.
   - Node IDs are prefixed: `user_<id>`, `subreddit_<id>`, `post_<id>`, `comment_<id>`.
   - Detailed content graph (posts/comments) gated by `DETAILED_GRAPH=true`.
   - `val` is stored as numeric text; `type` must be a stable enum string.

4. **Serve** — `GET /api/graph` prefers precalculated tables, falls back to legacy JSON.
   Server caps via `max_nodes` / `max_links` query params; caches 60s.
   Weighting: `max(Val, degree)` — keep this consistent if changing `GraphResponse`.

5. **Render** — Frontend fetches from `${VITE_API_URL || '/api'}/graph` and renders
   with `react-force-graph-3d`. Multiple views: 3D, 2D, Dashboard, Communities.
   Client honors caps via `VITE_MAX_RENDER_NODES` / `VITE_MAX_RENDER_LINKS`.

---

## 3 · API surface

The frontend depends on these contracts — do not break them without a migration plan.

| Method | Endpoint | Shape | Notes |
|--------|----------|-------|-------|
| `GET` | `/api/graph?max_nodes=20000&max_links=50000` | `{ nodes, links }` | Primary graph data |
| `GET` | `/api/communities?max_nodes=100&max_links=500&with_positions=true` | `{ nodes, links }` | Community supernodes |
| `GET` | `/api/communities/{id}?max_nodes=10000&max_links=50000` | `{ nodes, links }` | Community subgraph |
| `POST` | `/api/crawl` | `{"subreddit":"…"}` | Enqueue crawl job |
| `GET` | `/api/search`, `/api/export` | varied | Search + data export |
| `GET` | `/health` | `{"status":"ok"}` | Health check |
| `GET` | `/metrics` | Prometheus text | Prometheus scrape target |
| — | `/subreddits`, `/users`, `/posts`, `/comments`, `/jobs` | resource collections | No `/api` prefix |

Routes are registered in `backend/internal/api/routes.go`. Types live in
`frontend/src/types/graph.ts`.

---

## 4 · v2.0 Roadmap — active development plan

**Epic tracker:** Issue #138 — 6 epics, 9 dependency-ordered phases, ~50 sub-issues.

### Epics

| # | Epic | Key technologies | Issues |
|---|------|-----------------|--------|
| E1 | **Large-Scale Rendering Engine** | InstancedMesh, GPU lines, Web Workers, LOD, frustum culling, edge bundling | #145–#157 |
| E2 | **Backend Scalability & Streaming** | NDJSON streaming, tiered API, spatial queries, WebSocket, Redis cache, response compression | #158–#165 |
| E3 | **Graph Data Pipeline** | Multi-level Louvain, R-tree spatial index, edge bundle metadata, Barnes-Hut layout, incremental precalc | #167–#173 |
| E4 | **Frontend UX & Interaction** | Sidebar, minimap, search, inspector, onboarding, mobile/touch, keyboard nav, a11y, themes | #174–#183 |
| E5 | **Testing, CI/CD & Quality** | Perf benchmarks, bundle size CI gate, visual regression, k6 load tests, 80% test coverage, release automation | #184–#189 |
| E6 | **Operational Maturity** | SLOs/SLIs, alerting, runbooks, user docs, structured error codes, landing page | #190–#195 |

### Phase execution order

1. **Instrumentation & Foundation** — Measure first: FPS HUD, loading/error states, API error codes, compression, bundle CI gate, test coverage baseline.
2. **Core Rendering Rewrite** — InstancedMesh nodes, GPU line rendering, Web Worker physics, materialized views.
3. **Rendering Stabilization** — Octree culling, SDF text, enhanced physics, Barnes-Hut layout, Louvain upgrade.
4. **LOD, Spatial, Caching** — Level-of-detail, spatial hover, camera-distance scaling, R-tree, Redis cache.
5. **Streaming API** — Tiered API, NDJSON streaming, pagination, spatial viewport queries, multi-resolution precompute.
6. **Advanced Rendering + UX** — Edge bundling, sidebar, theme system, search with autocomplete.
7. **UX Polish + Perf Validation** — Inspector panel, minimap, keyboard nav, mobile support, perf benchmarks, k6 load tests.
8. **A11y, Visual Testing, Live** — WCAG 2.1 AA, visual regression testing, WebSocket live updates, SLOs.
9. **Launch Readiness** — Onboarding tour, alerting runbooks, operational docs, user help system, release pipeline, landing page.

### Working on v2.0 issues

When implementing a v2.0 sub-issue:

- Reference the parent epic and issue number in your PR title (e.g., `feat(E1): GPU-instanced node rendering (#145)`).
- Follow the **phase dependencies** — do not merge work from a later phase before its prerequisites land.
- Performance-sensitive changes (E1, E2, E3) must include **before/after benchmarks** in the PR description.
- Rendering changes must be tested at **10k, 50k, and 100k node counts**.
- New streaming or caching logic must handle **graceful degradation** (fall back to the existing JSON API).

---

## 5 · Conventions & patterns

### Go backend

- **Config** — `config.Load()` returns a memoized config struct. Call `config.ResetForTest()` in tests that modify env vars.
- **sqlc** — Edit SQL in `backend/internal/queries/*.sql`, then run `make generate`. Never edit `backend/internal/db/*.go` by hand.
- **Error handling** — Always wrap errors with `fmt.Errorf("context: %w", err)`. Use `context.Context` as the first parameter for blocking operations.
- **Rate limiting** — All Reddit API calls go through the global ticker in `internal/crawler/ratelimit.go`. Never bypass it.
- **HTTP retries** — Use `httpx` helpers. They handle `Retry-After`, exponential backoff, and circuit breaker.
- **Testing** — Table-driven tests. Integration tests live in `backend/internal/graph` and require `TEST_DATABASE_URL`.
- **Imports** — Order: stdlib → third-party → local. Module path: `github.com/subculture-collective/clustr/backend`.

### Frontend (React + TypeScript)

- **Language** — TypeScript only; no `any` types. Define interfaces for all data structures.
- **Components** — Functional components + hooks. Extract reusable logic into custom hooks.
- **Styling** — Tailwind CSS via `tailwind.config.js` + `postcss.config.js`.
- **Visualization** — `react-force-graph-3d` for 3D, D3.js for 2D. Node `type` field drives color mapping — keep it stable.
- **Testing** — Vitest for unit tests, Playwright in `frontend/e2e/` for E2E. Target 80% component coverage (E5 goal).
- **Build** — Vite 6 + TypeScript 5.8. Node 22.x. `npm ci` for CI; `npm install` for local dev.

### SQL & migrations

- golang-migrate files in `backend/migrations/`. Run `make migrate-up` (container) or `make migrate-up-host` (local).
- Lowercase SQL keywords in query files. Descriptive sqlc `-- name:` comments.
- Always consider index usage; run `EXPLAIN ANALYZE` for new queries.

### Commit messages

```
<type>(<scope>): <subject>

Types: feat, fix, docs, style, refactor, test, chore
Scopes: api, crawler, graph, frontend, config, ci, docs, deps
```

Reference issue numbers: `Closes #123` or `Part of #138`.

---

## 6 · Dev workflows

### Prerequisites

- Docker + Docker Compose (required)
- Go 1.24+ (local backend dev)
- Node.js 22+ (local frontend dev)
- `sqlc` and `golang-migrate` CLI (`make install-tools`)

### Common commands (from repo root)

| Command | What it does |
|---------|-------------|
| `make setup` | Create `.env`, install Go/npm deps |
| `make up` | Start all services (db, api, crawler, frontend, monitoring) |
| `make down` | Stop all services |
| `make rebuild` | Rebuild and restart all services |
| `make deploy` | Rebuild + run migrations |
| `make migrate-up` | Run migrations in container network |
| `make migrate-up-host` | Run migrations against localhost:5432 |
| `make generate` | Regenerate sqlc code |
| `make precalculate` | Run graph precalculation on-demand |
| `make test` | Run backend + frontend tests |
| `make test-backend` | Go tests only |
| `make test-frontend` | Frontend tests only |
| `make test-integration` | Integration tests (requires DB) |
| `make lint` | Run all linters (Go vet + gofmt + ESLint) |
| `make fmt` | Auto-format Go code |
| `make crawl SUB=AskReddit` | Enqueue a crawl job |
| `make status` | Show service status + DB + API health |
| `make logs` | Follow all service logs |
| `make db-shell` | Open psql shell |
| `make help` | List all available targets |

### Docker services

The `docker-compose.yml` in `backend/` brings up:
- `reddit-cluster-db` (Postgres 17, port 5432)
- `reddit-cluster-api` (Go API server, port 8000)
- `reddit-cluster-crawler` (Go crawler)
- `reddit-cluster-precalculate` (hourly precalc + backup)
- `reddit-cluster-frontend` (Nginx + Vite build)
- `reddit-cluster-prometheus` (port 9090)
- `reddit-cluster-grafana` (port 3000)
- `reddit-cluster-backup` (daily DB backups)

Network: `web` (external). All services use `restart: unless-stopped`.

### CI/CD

GitHub Actions workflows in `.github/workflows/`:
- `ci.yml` — Backend tests (Go + Postgres), frontend build (Node), Docker image builds
- `publish.yml` — Multi-arch image push to GHCR on tags
- `release.yml` — GitHub Release creation on tags
- `security.yml` — CodeQL and vulnerability scanning

---

## 7 · Adding or changing features

### New API endpoint

1. Write the handler in `backend/internal/api/handlers/`.
2. Register the route in `backend/internal/api/routes.go`.
3. If it needs new DB queries, add SQL in `backend/internal/queries/*.sql` and run `make generate`.
4. Return JSON with an explicit struct shape — no `map[string]interface{}`.
5. Add or update the corresponding TypeScript types in `frontend/src/types/`.
6. Document the endpoint in `docs/api.md`.

### Graph / precalculation changes

- Honor the ID prefix scheme (`user_<id>`, `subreddit_<id>`, etc.).
- Maintain capping logic (`max(Val, degree)` weighting).
- Ensure `val` (numeric text) and `type` (stable enum) are consistent.
- Community detection changes should update the multi-level Louvain in `internal/graph/`.

### Crawler changes

- All HTTP calls through the global rate limiter — no tight loops.
- Use `httpx` retry helpers with circuit breaker integration.
- Respect OAuth token lifecycle via `internal/authstore/`.

### Frontend rendering changes

- Keep the `type` field stable — it drives node color mapping across all views.
- Performance-critical rendering work (E1) must use `InstancedMesh` / GPU techniques.
- Test at 10k, 50k, and 100k nodes. Include FPS measurements in the PR.
- New components need corresponding Vitest unit tests.

### Database migrations

- Create new migration files via `migrate create -ext sql -dir backend/migrations -seq <name>`.
- Migrations must be reversible (provide both `up` and `down`).
- Test migrations on a fresh DB before merging.

---

## 8 · Environment variables (key subset)

| Variable | Default | Purpose |
|----------|---------|---------|
| `DATABASE_URL` | — | Postgres connection string |
| `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET` | — | Reddit OAuth |
| `REDDIT_USER_AGENT` | — | Reddit API user agent |
| `DETAILED_GRAPH` | `false` | Include posts/comments in graph |
| `POSTS_PER_SUB_IN_GRAPH` | `10` | Posts per subreddit in detailed graph |
| `COMMENTS_PER_POST_IN_GRAPH` | `50` | Comments per post in detailed graph |
| `MAX_AUTHOR_CONTENT_LINKS` | `3` | Cross-sub content links per author |
| `HTTP_MAX_RETRIES` | `3` | Max HTTP retries |
| `HTTP_TIMEOUT_MS` | `15000` | HTTP request timeout |
| `GRAPH_QUERY_TIMEOUT_MS` | `30000` | Graph API query timeout |
| `DB_STATEMENT_TIMEOUT_MS` | `25000` | DB statement timeout |
| `ENABLE_RATE_LIMIT` | `true` | API rate limiting |
| `CORS_ALLOWED_ORIGINS` | `localhost:5173,localhost:3000` | Allowed CORS origins |
| `ADMIN_API_TOKEN` | — | Bearer token for admin endpoints |
| `VITE_API_URL` | `/api` | Frontend API base URL |
| `VITE_MAX_RENDER_NODES` | — | Client-side node cap |
| `VITE_MAX_RENDER_LINKS` | — | Client-side link cap |

Full reference: `backend/internal/config/config.go` and `backend/.env.example`.

---

## 9 · Quality & review checklist

Before merging any PR:

- [ ] All CI checks pass (backend tests, frontend build, Docker builds, CodeQL)
- [ ] New code has tests (table-driven for Go, Vitest for React)
- [ ] No `any` types in TypeScript
- [ ] Errors are wrapped with context in Go
- [ ] sqlc regenerated if queries changed
- [ ] Migrations are reversible
- [ ] API changes documented and frontend types updated
- [ ] Commit messages follow convention
- [ ] Performance-sensitive changes include benchmarks
- [ ] No secrets, credentials, or API keys in code
