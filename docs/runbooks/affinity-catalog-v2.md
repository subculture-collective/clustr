# Affinity World and Catalog v2 release runbook

This runbook qualifies and promotes `spatial-scene-v2` together with one
immutable full-text Catalog. Building either artifact does not move a current
pointer. Promotion is an explicit, atomic operation; production remains on v1
until every gate below passes.

## Authority and fixed boundaries

- Never run the production section without a verified fresh Almaz backup,
  pinned backend/frontend/worker image digests, a reviewed Kvant rehearsal, and
  explicit production authorization.
- Pause the crawler only at the selected watermark boundary. Record that exact
  timestamp and use it for both shadow commands.
- OpenRouter credentials exist only in the publication worker environment.
  Never pass the API key on a command line or store it in a report/revision.
- Do not enable `CATALOG_ENABLED` or `VITE_CATALOG_ENABLED` until the coordinated
  promotion is complete. Tier 1 is the only initially enabled sitemap tier.
- Never run the down migration while any retained revision/catalog refers to
  the additive v2 objects.

## 1. Clone rehearsal on Kvant

1. Create and verify a fresh clone using
   [production-database-clone.md](production-database-clone.md). Record the
   clone backup checksum and source watermark.
2. Pin the candidate commit and all image digests. Run the migration entrypoint
   once against the isolated clone and verify that
   `000032_affinity_catalog_v2.up.sql` appears in
   `clustr_schema_migrations` with the repository checksum.
3. Confirm capacity before building. Free database storage must exceed the
   expected current-plus-previous document tables and indexes, temporary
   affinity workspace, and rollback headroom.
4. Export the label configuration only in the bounded worker environment:

   ```sh
   export CLUSTER_LABEL_BASE_URL='https://openrouter.ai/api/v1'
   export CLUSTER_LABEL_MODEL='<pinned-provider-model>'
   export CLUSTER_LABEL_API_KEY='<worker-secret>'
   ```

5. Build the immutable Catalog and affinity workspace at the same watermark:

   ```sh
   go run ./cmd/catalog-shadow --watermark '<RFC3339-watermark>'
   go run ./cmd/affinity-shadow --watermark '<RFC3339-watermark>'
   ```

   Both commands are bounded to one hour. The Catalog finishes in `published`
   state but does not become current; the affinity build finishes `validated`.
   Neither command changes `graph_revision_current` or
   `spatial_catalog_current`.

6. Record wall duration, peak worker RSS, catalog/database size, effective
   signal weights, provider model/prompt/policy versions, label cache and
   fallback rates, fine resolution/modularity, macro resolution/count, and all
   identity events.

## 2. Automated shadow gates

Fail the candidate if any condition is false:

- affinity worker RSS is below 4 GiB and both builds complete inside one hour;
- every affinity and coordinate is finite and every public link has two visible
  endpoints;
- the fine profile has at least 95% positive-evidence nodes in connected
  non-singletons, under 20% singletons, and median cluster size 5 through 500;
- there are 8 through 12 macro-groups and at least five overview landmarks per
  macro-group;
- matched landmark coordinates meet the 95% / 10% continuity gate, excluding
  recorded split/merge events;
- current and previous full-text catalogs remain retained;
- deleted/removed text is absent and every source/admin suppression disappears
  from graph, links, search, Catalog, HTML, JSON-LD, telemetry, and sitemap;
- top-300 deterministic fallback rate is below 5% and no accepted name is
  malformed, duplicated, unsupported, or dominated by one generic default;
- warm Catalog/search and node-detail p95 are under 300 ms; tree-child p95 is
  under 500 ms; existing overview/region budgets do not regress.

Run the complete repository gates:

```sh
cd backend && go test ./...
cd ../frontend && npm run lint && npm test -- --run && npm run build && npm run size
```

Also run the production-bundle browser suite at desktop, `390x844` portrait,
and mobile landscape against the real cloned Catalog. Capture screenshots,
console errors, failed network requests, keyboard traversal, reduced motion,
high contrast, and 44 px target checks. Compare semantic meaning—not raster
pixels—with both approved concept boards.

## 3. Review report

The reviewer packet must contain old/new revision and catalog IDs; exact visible
counts; old/new cluster sizes and representatives; tagged-default frequency;
fine and macro split/merge/new/retired facts; macro colors and overview
coverage; fallback/cache rates; source watermark; database size; durations/RSS;
route timings; desktop/mobile screenshots; console/network results; migration
ledger; backup checksum; candidate image/commit digests; and the exact promotion
and rollback commands.

## 4. Explicit coordinated promotion

After the report is approved, promote the validated build and matching Catalog:

```sh
go run ./cmd/promote-affinity \
  --build '<validated-build-id>' \
  --catalog '<published-shadow-catalog-id>' \
  --confirm-production-promotion
```

The command obtains the publication advisory lock, verifies the build and
Catalog watermarks are the same instant, publishes the v2 revision, and changes
both pointers in one serializable transaction. A failure rolls back the whole
transaction.

Deploy the digest-pinned compatible services and then enable both server and
frontend Catalog flags. Keep `SITEMAP_TIER2_ENABLED=false` and
`SITEMAP_TIER3_ENABLED=false` initially. Verify the manifest, telemetry,
overview, Catalog API, entity HTML, canonical/noindex rules, and Tier 1 sitemap
all report the same revision/catalog pair.

## 5. Monitoring and sitemap expansion

Monitor worker duration/RSS, database growth, affinity distributions,
cluster-size and macro-group churn, naming cache/provider/fallback rates, API
latency/errors, suppression hits, sensitive disclosures, crawler request load,
sitemap errors, frontend console/network failures, and blank-scene recovery.

Enable Tier 2 only after interactive budgets, crawler load, index coverage, and
error rates remain inside their gates. `SITEMAP_TIER2_ENTITY_LIMIT` controls the
activity-ranked user/post tranche. Enable Tier 3 only after a second review; it
contains the remaining non-sensitive users/posts and non-sensitive comments.

## 6. Atomic pointer rollback

Record the prior revision and its referenced Catalog before promotion. To
restore them:

```sh
go run ./cmd/rollback-affinity \
  --revision '<prior-published-revision-id>' \
  --catalog '<prior-referenced-catalog-id>' \
  --confirm-production-rollback
```

Then disable server/frontend v2 and Catalog flags, disable Tier 2/3 sitemaps,
and redeploy the pinned v1-compatible frontend. The rollback command refuses a
Catalog that is not the exact retained Catalog referenced by that revision.
Additive schema, audit history, and suppression records remain in place.
