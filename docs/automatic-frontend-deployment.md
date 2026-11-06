# Automatic frontend deployment

Native Gitea main CI deploys the frontend to Almaz after the existing backend,
frontend, browser, image-build and secret-scanning jobs pass. The deployment job
builds its exact main revision, validates nginx with isolated syntax-only host
mappings, and scans that actual image with the same Trivy severity policy as the
existing image job. PRs build and scan without activating production. A manual
main dispatch runs the same gates and deployment.

The build retains same-origin `/api` and `VITE_CATALOG_ENABLED=false`. This path
updates only `clustr-frontend`. Backend, crawler, database, shadow artifacts and
publication pointers keep their existing release procedures, including the
[Affinity World and Catalog v2 gates](runbooks/affinity-catalog-v2.md). It does
not enable Catalog or perform a coordinated v2 promotion.

An app-specific restricted key uses the private fleet-deploy receiver on Almaz.
The operator-owned frontend baseline retains its existing `projects` network,
restart policy, logging, environment overrides and nginx routing. No API env file
is read during frontend activation. The receiver refuses stale main revisions or
runtime drift, checks image identity, health and exact public HTML, and restores
the previous image if readiness fails. A retry reuses the first accepted image
for that commit rather than rebuilding it.

Gitea stores `HOMELAB_DEPLOY_KEY` and pinned `HOMELAB_KNOWN_HOSTS`. The homelab
operator owns this key; rotate it by 2027-02-03. Register and test the host profile
before merge. Private host receipts retain image IDs, diagnostics and rollback
commands. After deployment, compare public HTML/assets with the running image
and check `/api/health` and the existing readiness contract separately.
