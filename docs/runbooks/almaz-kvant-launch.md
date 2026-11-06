# Almaz + Kvant launch runbook

## Scope and safety boundary

Almaz remains the durable service and PostgreSQL host. Kvant runs one bounded,
CPU-first calculation at a time and publishes an immutable revision back to
Almaz. PostgreSQL stays bound to Almaz loopback; the worker opens a temporary
SSH tunnel and closes it after the run. The full Spatial Catalog is SQL-heavy:
placement, labeling, validation, and typed-link projection execute in
PostgreSQL on Almaz while Kvant owns the job lifecycle and the smaller
force-layout stage. A GPU would not materially improve this implementation;
use CPU until a measured native compute stage exists.

Each run records its start time as the source watermark and advances the
incremental cutoff only after a successful build. Rows arriving during a run
remain newer than that cutoff and are reconsidered on the next run. The mutable
workspace calculation is not yet one long PostgreSQL snapshot, so keep the
crawler disabled for the first cutover publication; readers are nevertheless
isolated by the atomic immutable-revision pointer.

Do not run the production section without an explicit cutover approval, a fresh
backup, and a pinned release image digest. Never down-migrate while a revision
is current. Keep the legacy read and crawler flags for the full revision
retention window.

## Qualified rehearsal baseline (2026-08-12)

The rehearsal used a verified custom-format clone of the Almaz `reddit_cluster`
database on Kvant. After the bounded crawler canary, the fixed source watermark
contained 320,554 subreddits, 1,866,126 users, 401,367 posts, and 4,306,542
comments: 6,894,589 source objects total. Migrations 28 through 30 completed,
every subreddit received a durable crawl request, and no request was missing.

A full CPU calculation and immutable publication completed in 593,249 ms with
597,672 KiB peak Go RSS. It produced revision 9 with 100,000 nodes, 200,000
typed weighted links, 6,794 communities, finite non-collapsed XYZ bounds, and
no orphan links. The revision-aware health, readiness, manifest, overview,
region, community, and diff routes returned successfully. Hardware-accelerated
Chromium rendered the real revision and keyboard travel plus inspection worked.

The full labeled Spatial Catalog then completed in 1,103,366 ms (18m 23s)
with 19,900 KiB peak Go-process RSS. Catalog 9 contains all 6,894,589 objects,
4,577,767 cross-cutting weighted links, exact per-type counts, no empty labels,
no orphan endpoints, and finite non-collapsed XYZ bounds. Its entity table and
indexes use 3,192 MB and its link table and indexes use 1,683 MB; the complete
rehearsal database uses 9,261 MB. Revision 11 atomically references that catalog
while keeping the GPU-resident overview capped at 100,000 nodes and 200,000
links. A 20-request warm camera-region sample at 1,000 nodes measured 142 ms
p95 (143 ms max); exact-entity search measured 1.2 ms and a common label-prefix
search 19.5 ms. Chromium searched for a real fallback-labeled comment, flew to
its persisted coordinate, rendered its label, restored camera state in the URL,
and reported zero console errors or warnings.

This is clone and local-browser evidence, not proof of a deployed release or a
live production deployment. A deliberately bounded Reddit canary did run
against the clone: `r/test` completed three consecutive generations, each with
retry number zero, one post and five comments persisted, next-due scheduling
advanced after every success, graceful shutdown completed, and no lease was
left expired. This proves the current OAuth and collection path but does not
replace a post-deploy canary on Almaz.

## One-time release preparation

1. Create and verify a current production clone by following
   [production-database-clone.md](production-database-clone.md). Run migrations
   and the full calculation against that isolated Kvant clone before scheduling
   a calculation against Almaz.
2. Build and test from a clean checkout whose `HEAD` is descended from the
   recorded `origin/main` base. Pass that exact `HEAD` as the worker image's
   `COMMIT` build argument. Never build a production image from a mutable
   checkout.
3. Push the image, resolve its immutable digest, and create a release bundle:

   ```bash
   deploy/kvant/build-worker-release.sh \
     "$PWD" "$HOME/.config/clustr/kvant-precalculate.env" \
     registry.example/precalculate@sha256:NEW \
     registry.example/precalculate@sha256:ROLLBACK \
     ORIGIN_MAIN_SHA "$HOME/.local/share/clustr/releases/RELEASE_ID"
   ```

   The command fails closed if the checkout is dirty, the image is not pinned,
   or `/app/precalculate --version` does not report the checkout SHA. Retain
   `release.env`, `patches.tsv`, `SHA256SUMS`, the extracted worker binary, the
   installed wrapper/units, and the runbook. `patches.tsv` is the complete
   compiled change list; do not substitute an informal release description.
4. Install the bundle's wrapper in `~/.local/libexec/clustr`, units in
   `~/.config/systemd/user`, and runbook in `~/.local/share/doc/clustr`. The
   normal Kvant deployment uses the user manager; root-owned unit templates are
   retained only for installations that deliberately use the system manager.
5. Create `~/.config/clustr/kvant-precalculate.env` from the example, owned by
   `onnwee` with mode `0600`. Set a percent-encoded database URL whose host is
   `127.0.0.1` and port is the configured tunnel port. Do not copy Reddit OAuth
   credentials into this file; the graph worker does not need them. Compare its
   checksum with `release.env` without printing the file.
6. Verify `ssh -F /dev/null -o BatchMode=yes 10.0.0.200 true` as `onnwee` and
   pin Almaz's host key before enabling the timer.
7. Validate without starting a calculation:

   ```bash
   systemd-analyze --user verify ~/.config/systemd/user/clustr-precalculate.service
   systemd-analyze --user verify ~/.config/systemd/user/clustr-precalculate.timer
   systemd-analyze --user verify ~/.config/systemd/user/clustr-spatial-catalog.service
   systemd-analyze --user verify ~/.config/systemd/user/clustr-spatial-catalog-preflight.service
   systemctl --user daemon-reload
   systemctl --user list-timers clustr-precalculate.timer
   ```

## Production migration and first publication

These are deliberate operator steps, not commands for unattended automation.

1. Leave the digest-pinned hourly graph worker running during preparation.
   For cutover, wait for its active run to finish, disable the user timer, and
   prove both the unit and any `clustr-precalculate-*` container are absent.
   Leave the current API/frontend serving the existing read model.
2. Take a fresh Almaz database backup and verify its checksum and archive TOC.
   Restore it to an isolated Kvant clone.
3. Production may physically contain migrations 29–36 while its immutable
   file ledger ends at 28. Never replay those migrations against that state.
   Run `scripts/production/reconcile-schema-29-36.sh ... audit` on the fresh
   clone, inspect its normalized schema, constraints, indexes, grants,
   partitions, and pointer snapshot, then reconcile the clone using the audit's
   schema fingerprint. Run the repository migration command and require it to
   report current without applying SQL. Run integration and full calculation
   gates on the clone. At production cutover, repeat the audit and reconcile
   only when its fingerprint exactly equals the qualified clone:

   ```bash
   scripts/production/reconcile-schema-29-36.sh \
     "$DATABASE_URL" backend/migrations audit /secure/evidence/prod-audit
   scripts/production/reconcile-schema-29-36.sh \
     "$DATABASE_URL" backend/migrations reconcile \
     QUALIFIED_CLONE_SCHEMA_SHA256 /secure/evidence/prod-reconcile
   ```

   Verify the graph revision and catalog pointers are unchanged before and
   after reconciliation. Any mismatch requires a new additive migration.
4. Keep `CRAWLER_LIFECYCLE_ENABLED=false` for the first API restart. Keep
   revision reads disabled until the first immutable revision is present if the
   deployment health orchestration cannot tolerate a deliberately unready API.
5. Install the clean release bundle and run one graph-only canary. It must use
   the already-published catalog and must not build or move a catalog:

   ```bash
   ~/.local/libexec/clustr/run-precalculation.sh \
     ~/.config/clustr/kvant-precalculate.env graph-revision
   ```

6. The canary must exit within one hour and remain below 4 GiB peak RSS. On
   Almaz, verify all of the following
   before switching reads:
   - the current revision pointer advanced exactly once;
   - node/link/community counts are within configured caps;
   - the catalog pointer is still the pre-canary value;
   - no link has a missing endpoint;
   - coordinates and bounds are finite and non-collapsed;
   - `/ready` reports schema, crawl leases, and publication as healthy;
   - manifest, overview, bounded region, community, entity, search, node-detail,
     and diff responses all name the same revision and spatial catalog.
7. Enable revision reads and restart only the API/frontend services needed for
   the flag change. Validate the public route in hardware-accelerated Chromium:
   meaningful separated landmarks, camera travel, selection, inspector,
   minimap heading, responsive layout, reduced motion, and no console/network
   errors.
8. Enable the Kvant timer only after the manual publication passes, then retain
   evidence for three consecutive successful scheduled runs:

   ```bash
   systemctl --user enable --now clustr-precalculate.timer
   systemctl --user list-timers clustr-precalculate.timer
   ```

The catalog preflight and rebuild units are static oneshots. They are never
enabled and have no timer. Qualify `catalog-rebuild` separately on a fresh
production clone; one hour, 4 GiB peak RSS, disk headroom, exact document
coverage, orphan rejection, and pointer safety are hard gates.

## Crawler cutover

The migration imports the entire subreddit corpus into durable requests, but
the imported cohort is intentionally throttled. Start with the new lifecycle
disabled, then perform one controlled Reddit canary against a staging clone.
It must demonstrate lease heartbeat, complete or partial outcome, next-due
scheduling, and a second recurrence after success. Test a retryable failure and
confirm the next generation starts again at retry zero.

After the canary, enable `CRAWLER_LIFECYCLE_ENABLED=true` on one Almaz crawler.
The default imported-backlog gate is 24 claims/hour; explicit API/manual work
remains responsive. Raise the gate only from observed Reddit quota, job
duration, partial/failure rate, and database load. Monitor expired leases and
daily discovery promotion budgets continuously.

## Rollback

- Calculation failure: do nothing to the pointer. Readers retain the previous
  immutable revision. Inspect the failed/staging revision and journal before a
  retry.
- Renderer/read failure: set `REVISION_READS_ENABLED=false`, restart the API,
  and use the legacy read Adapter. Do not drop revision tables.
- Crawler failure: set `CRAWLER_LIFECYCLE_ENABLED=false` and restart the crawler;
  retain durable attempt history and legacy job tables read-only.
- Bad new revision: after validating the prior revision artifacts, atomically
  repoint `graph_current_revision` in an operator transaction. There is not yet
  a dedicated rollback CLI, so record the old/new revision IDs and SQL result.
- Stop future calculations with
  `systemctl --user disable --now clustr-precalculate.timer`. A running oneshot may
  be stopped independently; its signal-aware worker exits and leaves the prior
  published pointer intact unless promotion already committed.

## Evidence to retain per release

Record the commit and image digests, migration ledger, backup checksum, source
watermark, revision ID/counts/bounds, calculation duration/peak RSS, readiness
JSON, route timings, crawler canary attempt IDs (without secrets), browser
screenshots, console/network logs, and rollback flag values.
