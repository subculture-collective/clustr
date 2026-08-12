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

1. Build and test the exact release checkout. Build all server, crawler,
   precalculate, migration, and frontend images from the same commit.
2. Push immutable images and record their digests. Never use `latest` in the
   Kvant unit.
3. Copy this checkout to `/opt/clustr` on Kvant, preserving ownership by the
   `onnwee` user. Install the unit and timer from `deploy/kvant/`.
4. Create `/etc/clustr/kvant-precalculate.env` from the example, owned by
   `root:onnwee` with mode `0640`. Set a percent-encoded database URL whose host is
   `127.0.0.1` and port is the configured tunnel port. Do not copy Reddit OAuth
   credentials into this file; the graph worker does not need them.
5. Verify `ssh -o BatchMode=yes almaz true` as the service user and pin Almaz's
   host key before enabling the timer.
6. Validate without starting a calculation:

   ```bash
   sudo systemd-analyze verify /etc/systemd/system/clustr-precalculate.service
   sudo systemd-analyze verify /etc/systemd/system/clustr-precalculate.timer
   sudo systemctl daemon-reload
   sudo systemctl list-timers clustr-precalculate.timer
   ```

## Production migration and first publication

These are deliberate operator steps, not commands for unattended automation.

1. Stop the old precalculate service if one exists. Leave the current API and
   frontend serving their existing read model.
2. Take a fresh Almaz database backup and verify its checksum and archive TOC.
3. Run the migration image once against Almaz. Verify the migration ledger
   contains `000028_crawl_lifecycle.up.sql`,
   `000029_graph_revisions.up.sql`, and
   `000030_spatial_catalog.up.sql`. Confirm `crawl_requests` has one row per
   intended subreddit and inspect the active/backlog cohort counts.
4. Keep `CRAWLER_LIFECYCLE_ENABLED=false` for the first API restart. Keep
   revision reads disabled until the first immutable revision is present if the
   deployment health orchestration cannot tolerate a deliberately unready API.
5. Run the first calculation manually on Kvant in initial-full mode and follow
   its journal. This mode calculates the resident graph and the full labeled
   catalog. Ordinary hourly runs use the same complete contract at a new source
   watermark; they do not leave the catalog frozen at launch:

   ```bash
   sudo -u onnwee /opt/clustr/deploy/kvant/run-precalculation.sh \
     /etc/clustr/kvant-precalculate.env --initial-full \
     2>&1 | tee /var/tmp/clustr-initial-full.log
   ```

6. The service must exit successfully. On Almaz, verify all of the following
   before switching reads:
   - the current revision pointer advanced exactly once;
   - node/link/community counts are within configured caps;
   - catalog entity counts exactly match the four source entity tables at the
     recorded watermark and every entity has a nonempty semantic label;
   - catalog link endpoints are complete and catalog bounds are finite and
     non-collapsed;
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
8. Enable the Kvant timer only after the manual publication passes:

   ```bash
   sudo systemctl enable --now clustr-precalculate.timer
   systemctl list-timers clustr-precalculate.timer
   ```

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
  `sudo systemctl disable --now clustr-precalculate.timer`. A running oneshot may
  be stopped independently; its signal-aware worker exits and leaves the prior
  published pointer intact unless promotion already committed.

## Evidence to retain per release

Record the commit and image digests, migration ledger, backup checksum, source
watermark, revision ID/counts/bounds, calculation duration/peak RSS, readiness
JSON, route timings, crawler canary attempt IDs (without secrets), browser
screenshots, console/network logs, and rollback flag values.
