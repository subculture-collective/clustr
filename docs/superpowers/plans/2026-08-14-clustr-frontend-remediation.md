# Clustr Frontend Remediation Plan

## Summary

Replace the remaining sample-derived public views with immutable, revision-pinned published-world contracts; make every primary view share one world pin for the page lifetime; and close the rendered interaction, responsive, accessibility, metadata, and test-gate findings from the Chrome review of `clustr.subcult.tv`.

Clustr is pre-launch and downtime is acceptable. Ship the backend and frontend as one coordinated release, without temporary feature hiding. No database migration is expected: the published graph revision and spatial catalog remain the source of truth.

## Implementation changes

- Add `GET /api/graph/telemetry?revision=&top_limit=` for exact catalog/entity/link/community counts, exact per-type totals, top subreddits, and most-active users from the pinned spatial catalog.
- Add `GET /api/graph/communities?revision=&level=0&limit=&cursor=` for a stable, paginated list of published community landmarks. Bind continuation tokens to revision and level.
- Keep `/api/nodes/{id}` revision-aware for subreddit, user, post, comment, and community identities, and include revision/catalog identity in details responses.
- Extend `SpatialSceneClient` and introduce a page-scoped provider so Universe, Map, Data, Places, search, and inspector requests share one immutable revision/catalog pin.
- Rebuild Map as a direct 2D projection of the bounded published overview. Remove its bulk legacy graph request and force simulation.
- Rebuild Data from exact telemetry and remove sample averages, sample degree claims, and all other metrics that look corpus-wide but are derived from a client sample.
- Rebuild Places from the published community catalog. Remove public Louvain recomputation and derive display colors deterministically from stable community IDs.
- Stop repeated inspector requests, display readable selection labels, let Enter open the full inspector, and make World view reset selection and camera to the published overview.
- Run Troika text generation without a worker, keep labels revision-consistent, and expose the rendered label count for production verification.
- Resolve mobile overlap/tap-zone issues, keyboard/focus semantics, reduced-motion behavior, contrast and accessible naming findings across primary views.
- Complete canonical, Open Graph, Twitter, manifest, icon, and social-card metadata. A user-visible social-card render remains subject to explicit visual approval before publication.
- Make frontend lint a required CI gate and remove the Octree unit test's wall-clock assertion; keep performance budgets in the dedicated browser benchmark suite.

## Test and release qualification

- Backend contract tests cover revision pinning, exact totals, deterministic ordering, pagination/cursor scope, invalid revisions, and community node details.
- Frontend tests cover one manifest pin per page, no bulk graph calls from Map/Data/Places, published landmark rendering, deterministic colors, inspector request stability, keyboard inspection, World view reset, mobile layout, accessibility, and metadata.
- Run backend tests, frontend unit/coverage/lint/build checks, production-bundle Playwright tests at desktop and mobile viewports, and `git diff --check`.
- Qualify the built image with commit/version provenance, then repeat the live Chrome acceptance pass after deployment. Local success is not deployment evidence.

## Locked assumptions

- Published-world views are authoritative; public Places recomputation is removed.
- A page pins one revision for its lifetime. Refreshing the page is the explicit way to adopt a newer publication.
- Backend and frontend deploy together; no compatibility shim for independently deployed versions is required.
- No commit, push, deployment, production mutation, or social-card publication occurs without separate authorization.
