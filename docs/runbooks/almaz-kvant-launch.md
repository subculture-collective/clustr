# Almaz + Kvant launch runbook

## Scope and safety boundary

Almaz is the PostgreSQL host; Kvant performs one bounded calculation at a time
through a temporary SSH tunnel. PostgreSQL remains loopback-only on Almaz.
`graph-revision` is the only scheduled mode. The catalog services are both
static and manual-only: `clustr-spatial-catalog` performs a real
`catalog-rebuild`; `clustr-spatial-catalog-preflight` only checks capacity.
There are **no catalog timers**.

The canonical deployment is the `onnwee` user manager. The tracked
system-manager templates are inventory/review counterparts, not an approved
alternative in this release: do not install them or substitute equivalent
system commands.

The current catalog gate is below threshold (about 28 GiB free; minimum 40
GiB on the actual `/mnt/spektr/` PostgreSQL bind-source). Do not start a
catalog rebuild or modify Hasanara data. Start only the preflight service; it
checks capacity and exits without launching a rebuild.

## Release provenance and SSH prerequisites

Build the image and wrapper in an isolated clean worktree at base `6eb5010`,
with only the approved reviewed patches. Do not build, copy, or execute from a
mutable project checkout. Before installation, prepare a concrete user-owned
release from that clean build artifact (replace `CLEAN_ARTIFACT` with its
absolute path):

```bash
BASE_SHA=6eb5010
RELEASE_ID="${BASE_SHA}-reviewed-$(date -u +%Y%m%dT%H%M%SZ)"
RELEASE_DIR="/home/onnwee/.local/share/clustr/releases/${RELEASE_ID}"
CLEAN_ARTIFACT=/absolute/path/to/isolated-clean-build-artifact
PATCH_MANIFEST=/absolute/path/to/approved-reviewed-patch-manifest

test -d "$CLEAN_ARTIFACT"
test -f "$PATCH_MANIFEST"
install -d -m 0755 "$RELEASE_DIR"
install -m 0444 "$PATCH_MANIFEST" "$RELEASE_DIR/approved-reviewed-patch-manifest"
install -m 0755 "$CLEAN_ARTIFACT/deploy/kvant/run-precalculation.sh" "$RELEASE_DIR/run-precalculation.sh"
install -m 0644 "$CLEAN_ARTIFACT/deploy/kvant/kvant-precalculate.env.example" "$RELEASE_DIR/kvant-precalculate.env.example"
install -m 0644 "$CLEAN_ARTIFACT/docs/runbooks/almaz-kvant-launch.md" "$RELEASE_DIR/almaz-kvant-launch.md"
install -m 0644 "$CLEAN_ARTIFACT/deploy/kvant/clustr-precalculate.user.service" "$RELEASE_DIR/clustr-precalculate.user.service"
install -m 0644 "$CLEAN_ARTIFACT/deploy/kvant/clustr-precalculate.user.timer" "$RELEASE_DIR/clustr-precalculate.user.timer"
install -m 0644 "$CLEAN_ARTIFACT/deploy/kvant/clustr-spatial-catalog.user.service" "$RELEASE_DIR/clustr-spatial-catalog.user.service"
install -m 0644 "$CLEAN_ARTIFACT/deploy/kvant/clustr-spatial-catalog-preflight.user.service" "$RELEASE_DIR/clustr-spatial-catalog-preflight.user.service"
printf 'base_sha=%s\n' "$BASE_SHA" >"$RELEASE_DIR/manifest"
printf '%s\n' 'reviewed_patch_name=REPLACE_WITH_APPROVED_PATCH' >>"$RELEASE_DIR/manifest"
printf '%s\n' 'reviewed_patch_sha256=REPLACE_WITH_APPROVED_PATCH_SHA256' >>"$RELEASE_DIR/manifest"
printf '%s\n' 'clustr_precalculate_image=REPLACE_WITH_DIGEST_PINNED_IMAGE_REFERENCE' >>"$RELEASE_DIR/manifest"
sha256sum "$RELEASE_DIR/run-precalculation.sh" \
  "$RELEASE_DIR/approved-reviewed-patch-manifest" \
  "$RELEASE_DIR/kvant-precalculate.env.example" \
  "$RELEASE_DIR/almaz-kvant-launch.md" \
  "$RELEASE_DIR/clustr-precalculate.user.service" \
  "$RELEASE_DIR/clustr-precalculate.user.timer" \
  "$RELEASE_DIR/clustr-spatial-catalog.user.service" \
  "$RELEASE_DIR/clustr-spatial-catalog-preflight.user.service" >>"$RELEASE_DIR/manifest"
chmod 0555 "$RELEASE_DIR"
chmod 0444 "$RELEASE_DIR"/*
```

Replace every `REPLACE_WITH...` value with the approved value before the final
checksum command; retain the resulting manifest as release evidence. It records
the clean base SHA, the checksum-recorded approved patch manifest (which lists
each reviewed patch name/SHA-256), the immutable image reference/digest, and
SHA-256 values for the wrapper and each release artifact.
The directory and its contents are non-writable before anything is installed.

Install and edit the environment outside the immutable release. It must record
its pathname and SHA-256 in the final installation record, never its secrets:

```bash
install -d -m 0700 /home/onnwee/.config/clustr
install -m 0600 "$RELEASE_DIR/kvant-precalculate.env.example" \
  /home/onnwee/.config/clustr/kvant-precalculate.env
editor /home/onnwee/.config/clustr/kvant-precalculate.env
```

The installed environment must set `ALMAZ_SSH_TARGET=10.0.0.200`, a
digest-pinned `CLUSTR_PRECALCULATE_IMAGE`, and the wrapper's loopback tunnel
database URL. The default identity is `/home/onnwee/.ssh/id_*`; if it is not
the authorized Almaz key, install the authorized default identity first. The
wrapper invokes SSH with `-F /dev/null`, `BatchMode=yes`, `ControlMaster=no`,
`ControlPath=none`, and `ControlPersist=no`, so `~/.ssh/config` aliases,
IdentityFile settings, and multiplexing cannot satisfy this prerequisite.

Pin Almaz’s host key in `/home/onnwee/.ssh/known_hosts` for `10.0.0.200`
(verify the fingerprint out of band). This non-interactive validation proves the
pin/default identity, remote Docker access, PostgreSQL container, and loopback
listener using the wrapper's strict SSH options:

```bash
ssh -F /dev/null -T -o BatchMode=yes -o ControlMaster=no -o ControlPath=none \
  -o ControlPersist=no -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile=/home/onnwee/.ssh/known_hosts 10.0.0.200 \
  'docker inspect pg17-clustr >/dev/null && bash -ec "</dev/tcp/127.0.0.1/15435"'
```

`pg17-clustr` and port `15435` must match the installed environment/release.
Do not relax host-key checking, use an SSH alias, or use an externally exposed
database listener.

## Staged installation

Before installation/reload, quiesce and verify **both managers**. This is also
required before rollback: static services can still be active even when no timer
is enabled. Do not rely only on the current wrapper lock: a legacy wrapper can
hold a different lock. Retain this output as release evidence.

```bash
set -euo pipefail
units=(clustr-precalculate.timer clustr-precalculate.service \
  clustr-spatial-catalog.service clustr-spatial-catalog-preflight.service)
quiesce_manager() {
  local manager=$1 unit load_state active_state
  shift
  local -a ctl=("$@")
  for unit in "${units[@]}"; do
    if ! load_state=$("${ctl[@]}" show --property=LoadState --value "$unit"); then
      printf '%s manager error reading LoadState for %s\n' "$manager" "$unit" >&2; return 1
    fi
    if [[ $load_state == not-found ]]; then
      printf '%s unit absent (LoadState=not-found): %s\n' "$manager" "$unit"
      continue
    fi
    if [[ $unit == clustr-precalculate.timer ]] && ! "${ctl[@]}" disable --now "$unit"; then
      printf '%s manager failed disabling %s\n' "$manager" "$unit" >&2; return 1
    fi
    if ! "${ctl[@]}" stop "$unit"; then
      printf '%s manager failed stopping %s\n' "$manager" "$unit" >&2; return 1
    fi
    if ! active_state=$("${ctl[@]}" show --property=ActiveState --value "$unit"); then
      printf '%s manager error reading ActiveState for %s\n' "$manager" "$unit" >&2; return 1
    fi
    case $active_state in
      inactive|failed) printf '%s unit stopped: %s (ActiveState=%s)\n' "$manager" "$unit" "$active_state" ;;
      *) printf '%s unit not quiescent: %s (ActiveState=%s)\n' "$manager" "$unit" "$active_state" >&2; return 1 ;;
    esac
  done
}
quiesce_manager user systemctl --user
quiesce_manager system sudo systemctl
systemctl --user list-timers --all clustr-precalculate.timer
sudo systemctl list-timers --all clustr-precalculate.timer

# A running legacy wrapper may not use the current precalculation.lock.
if processes=$(pgrep -af 'clustr-precalculate|run-precalculation'); then
  printf '%s\n' "$processes" >&2
  printf '%s\n' 'Clustr worker/wrapper process remains; investigate and stop it.' >&2; exit 1
else
  pgrep_status=$?
  (( pgrep_status == 1 )) || { printf 'pgrep failed: %s\n' "$pgrep_status" >&2; exit 1; }
fi
if ! containers=$(docker ps -a --format '{{.Names}}'); then
  printf '%s\n' 'docker ps failed; cannot prove worker-container quiescence.' >&2; exit 1
fi
while IFS= read -r container; do
  if [[ $container == clustr-precalculate* ]]; then
    printf 'Clustr worker container remains: %s\n' "$container" >&2; exit 1
  fi
done <<<"$containers" # An empty inventory (no matching worker container) is required.
```

Install only from the immutable `RELEASE_DIR` prepared above. These commands do
not require local sudo:

```bash
install -d -m 0700 /home/onnwee/.local/state/clustr
install -d -m 0755 /home/onnwee/.local/libexec/clustr /home/onnwee/.local/share/doc/clustr
install -m 0755 "$RELEASE_DIR/run-precalculation.sh" /home/onnwee/.local/libexec/clustr/run-precalculation.sh
install -m 0644 "$RELEASE_DIR/almaz-kvant-launch.md" /home/onnwee/.local/share/doc/clustr/almaz-kvant-launch.md
install -D -m 0644 "$RELEASE_DIR/clustr-precalculate.user.service" /home/onnwee/.config/systemd/user/clustr-precalculate.service
install -D -m 0644 "$RELEASE_DIR/clustr-precalculate.user.timer" /home/onnwee/.config/systemd/user/clustr-precalculate.timer
install -D -m 0644 "$RELEASE_DIR/clustr-spatial-catalog.user.service" /home/onnwee/.config/systemd/user/clustr-spatial-catalog.service
install -D -m 0644 "$RELEASE_DIR/clustr-spatial-catalog-preflight.user.service" /home/onnwee/.config/systemd/user/clustr-spatial-catalog-preflight.service
sha256sum /home/onnwee/.local/libexec/clustr/run-precalculation.sh \
  /home/onnwee/.local/share/doc/clustr/almaz-kvant-launch.md \
  /home/onnwee/.config/systemd/user/clustr-precalculate.service \
  /home/onnwee/.config/systemd/user/clustr-precalculate.timer \
  /home/onnwee/.config/systemd/user/clustr-spatial-catalog.service \
  /home/onnwee/.config/systemd/user/clustr-spatial-catalog-preflight.service \
  /home/onnwee/.config/clustr/kvant-precalculate.env \
  >/home/onnwee/.local/state/clustr/"${RELEASE_ID}"-installed-sha256
systemctl --user daemon-reload
```

The state-file `"${RELEASE_ID}"-installed-sha256` is written only after the
release was made immutable; retain it alongside the release record. Its
environment-file checksum identifies the installed file without recording its
contents. Check lingering separately; it is the other administrator action if
needed:

```bash
loginctl show-user onnwee -p Linger
# Administrator only, if required: sudo loginctl enable-linger onnwee
```

Require `Linger=yes`. Services are static—never enable either catalog service.
The system timer stays disabled; do not install system units in this rollout.

## Validate, preflight, and canary

```bash
systemd-analyze --user verify ~/.config/systemd/user/clustr-precalculate.service
systemd-analyze --user verify ~/.config/systemd/user/clustr-precalculate.timer
systemd-analyze --user verify ~/.config/systemd/user/clustr-spatial-catalog.service
systemd-analyze --user verify ~/.config/systemd/user/clustr-spatial-catalog-preflight.service
systemctl --user show clustr-precalculate.timer -p TimersMonotonic -p RandomizedDelayUSec
systemctl --user cat clustr-precalculate.service
systemctl --user cat clustr-spatial-catalog.service
systemctl --user cat clustr-spatial-catalog-preflight.service
systemctl --user list-timers --all clustr-precalculate.timer
systemctl --user start clustr-spatial-catalog-preflight.service
journalctl --user -u clustr-spatial-catalog-preflight.service -n 100 --no-pager
```

Confirm the preflight journal reports the below-threshold refusal and no worker
container/rebuild. Do **not** start `clustr-spatial-catalog.service` until disk
capacity is remediated and the catalog gate is satisfied. Confirm the graph unit
has 2h/2min timeouts, rebuild has 4h/5min, preflight has 2min/30s, all user
services use the shared lock directory. The graph timer bootstraps one hour
after activation, then runs one hour after the previous run becomes inactive,
with five-minute jitter and `Persistent=false`, so the two-hour readiness window
is met without overlap.

After a fresh verified Almaz backup, verify the installed image against the
release manifest and capture a machine-readable baseline **before** the canary.
`clustr_precalculate_image` is the release-manifest key for the approved
immutable image reference (including its `@sha256:` digest). Do not activate a
timer if either comparison or baseline query fails.

```bash
set -euo pipefail
ENV_FILE=/home/onnwee/.config/clustr/kvant-precalculate.env
expected_image=$(awk -F= '$1 == "clustr_precalculate_image" {print substr($0, index($0, "=") + 1); exit}' "$RELEASE_DIR/manifest")
installed_image=$(awk -F= '$1 == "CLUSTR_PRECALCULATE_IMAGE" {print substr($0, index($0, "=") + 1); exit}' "$ENV_FILE")
[[ $expected_image =~ @sha256:[0-9a-f]{64}$ && $installed_image == "$expected_image" ]]

ALMAZ_SSH_TARGET=$(awk -F= '$1 == "ALMAZ_SSH_TARGET" {print $2; exit}' "$ENV_FILE")
ALMAZ_POSTGRES_CONTAINER=$(awk -F= '$1 == "ALMAZ_POSTGRES_CONTAINER" {print $2; exit}' "$ENV_FILE")
: "${ALMAZ_SSH_TARGET:=10.0.0.200}" "${ALMAZ_POSTGRES_CONTAINER:=pg17-clustr}"
BASELINE_FILE="/home/onnwee/.local/state/clustr/${RELEASE_ID}-canary-baseline.tsv"
ssh -F /dev/null -T -o BatchMode=yes -o ControlMaster=no -o ControlPath=none \
  -o ControlPersist=no -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile=/home/onnwee/.ssh/known_hosts "$ALMAZ_SSH_TARGET" \
  "docker exec -i \"$ALMAZ_POSTGRES_CONTAINER\" bash -ceu 'exec psql -X -A -t -v ON_ERROR_STOP=1 -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\"'" \
  >"$BASELINE_FILE" <<'SQL'
SELECT grc.revision_id || E'\t' || scc.catalog_id || E'\t' || (SELECT max(id) FROM graph_revisions) || E'\t' ||
       gr.node_count || E'\t' || gr.link_count || E'\t' || gr.community_count
  FROM graph_revision_current grc
  CROSS JOIN spatial_catalog_current scc
  JOIN graph_revisions gr ON gr.id = grc.revision_id
 WHERE grc.singleton AND scc.singleton;
SQL
IFS=$'\t' read -r BASELINE_REVISION BASELINE_CATALOG BASELINE_MAX_REVISION \
  BASELINE_NODES BASELINE_LINKS BASELINE_COMMUNITIES <"$BASELINE_FILE"
[[ $BASELINE_REVISION =~ ^[0-9]+$ && $BASELINE_CATALOG =~ ^[0-9]+$ && $BASELINE_MAX_REVISION =~ ^[0-9]+$ && \
   $BASELINE_NODES =~ ^[0-9]+$ && $BASELINE_LINKS =~ ^[0-9]+$ && $BASELINE_COMMUNITIES =~ ^[0-9]+$ ]]
printf 'baseline revision=%s catalog=%s max_revision=%s nodes=%s links=%s communities=%s\n' \
  "$BASELINE_REVISION" "$BASELINE_CATALOG" "$BASELINE_MAX_REVISION" \
  "$BASELINE_NODES" "$BASELINE_LINKS" "$BASELINE_COMMUNITIES"
```

Run exactly one graph-only canary:

```bash
systemctl --user start clustr-precalculate.service
journalctl --user -u clustr-precalculate.service -n 200 --no-pager
```

Record clean image/script provenance and the canary journal. Then run this
closed-failure verification. It requires exactly one new revision, makes that
revision current, keeps the current catalog and attachment unchanged, and checks
published/valid/coherent status, exact baseline node/link/community counts,
configured caps, finite coordinates and bounds, and orphan links. A cap of `0`
means no cap for that type. It emits one TSV result row and retains it as
timestamped evidence.

```bash
set -euo pipefail
EVIDENCE_FILE="/home/onnwee/.local/state/clustr/${RELEASE_ID}-canary-validation-$(date -u +%Y%m%dT%H%M%SZ).tsv"
for cap in GRAPH_REVISION_NODE_CAP GRAPH_REVISION_LINK_CAP \
  GRAPH_REVISION_SUBREDDIT_CAP GRAPH_REVISION_USER_CAP \
  GRAPH_REVISION_POST_CAP GRAPH_REVISION_COMMENT_CAP; do
  value=$(awk -F= -v key="$cap" '$1 == key {print $2; exit}' "$ENV_FILE")
  [[ $value =~ ^[0-9]+$ ]] || { printf 'missing/invalid %s\n' "$cap" >&2; exit 1; }
  export "$cap=$value"
done
ssh -F /dev/null -T -o BatchMode=yes -o ControlMaster=no -o ControlPath=none \
  -o ControlPersist=no -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile=/home/onnwee/.ssh/known_hosts "$ALMAZ_SSH_TARGET" \
  "docker exec -i -e BASELINE_REVISION=$BASELINE_REVISION -e BASELINE_CATALOG=$BASELINE_CATALOG -e BASELINE_MAX_REVISION=$BASELINE_MAX_REVISION -e BASELINE_NODES=$BASELINE_NODES -e BASELINE_LINKS=$BASELINE_LINKS -e BASELINE_COMMUNITIES=$BASELINE_COMMUNITIES -e NODE_CAP=$GRAPH_REVISION_NODE_CAP -e LINK_CAP=$GRAPH_REVISION_LINK_CAP -e SUBREDDIT_CAP=$GRAPH_REVISION_SUBREDDIT_CAP -e USER_CAP=$GRAPH_REVISION_USER_CAP -e POST_CAP=$GRAPH_REVISION_POST_CAP -e COMMENT_CAP=$GRAPH_REVISION_COMMENT_CAP \"$ALMAZ_POSTGRES_CONTAINER\" bash -ceu 'exec psql -X -A -t -v ON_ERROR_STOP=1 -v baseline_revision=\"\$BASELINE_REVISION\" -v baseline_catalog=\"\$BASELINE_CATALOG\" -v baseline_max_revision=\"\$BASELINE_MAX_REVISION\" -v baseline_nodes=\"\$BASELINE_NODES\" -v baseline_links=\"\$BASELINE_LINKS\" -v baseline_communities=\"\$BASELINE_COMMUNITIES\" -v node_cap=\"\$NODE_CAP\" -v link_cap=\"\$LINK_CAP\" -v subreddit_cap=\"\$SUBREDDIT_CAP\" -v user_cap=\"\$USER_CAP\" -v post_cap=\"\$POST_CAP\" -v comment_cap=\"\$COMMENT_CAP\" -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\"'" <<'SQL' | tee "$EVIDENCE_FILE"
SELECT set_config('clustr.canary.baseline_revision', :'baseline_revision', false) \g /dev/null
SELECT set_config('clustr.canary.baseline_catalog', :'baseline_catalog', false) \g /dev/null
SELECT set_config('clustr.canary.baseline_max_revision', :'baseline_max_revision', false) \g /dev/null
SELECT set_config('clustr.canary.baseline_nodes', :'baseline_nodes', false) \g /dev/null
SELECT set_config('clustr.canary.baseline_links', :'baseline_links', false) \g /dev/null
SELECT set_config('clustr.canary.baseline_communities', :'baseline_communities', false) \g /dev/null
SELECT set_config('clustr.canary.node_cap', :'node_cap', false) \g /dev/null
SELECT set_config('clustr.canary.link_cap', :'link_cap', false) \g /dev/null
SELECT set_config('clustr.canary.subreddit_cap', :'subreddit_cap', false) \g /dev/null
SELECT set_config('clustr.canary.user_cap', :'user_cap', false) \g /dev/null
SELECT set_config('clustr.canary.post_cap', :'post_cap', false) \g /dev/null
SELECT set_config('clustr.canary.comment_cap', :'comment_cap', false) \g /dev/null
DO $$
DECLARE
  old_revision bigint := current_setting('clustr.canary.baseline_revision')::bigint; old_catalog bigint := current_setting('clustr.canary.baseline_catalog')::bigint; old_max bigint := current_setting('clustr.canary.baseline_max_revision')::bigint;
  expected_nodes bigint := current_setting('clustr.canary.baseline_nodes')::bigint; expected_links bigint := current_setting('clustr.canary.baseline_links')::bigint; expected_communities bigint := current_setting('clustr.canary.baseline_communities')::bigint;
  new_revision bigint; attached_catalog bigint; revision_status text; catalog_status text; validation jsonb; catalog_validation jsonb;
  nodes bigint; links bigint; communities bigint; subreddits bigint; users bigint; posts bigint; comments bigint; orphan_links bigint; catalog_orphan_links bigint;
BEGIN
  SELECT id, spatial_catalog_id, status, validation_result INTO new_revision, attached_catalog, revision_status, validation
    FROM graph_revisions WHERE id > old_max ORDER BY id;
  IF (SELECT count(*) FROM graph_revisions WHERE id > old_max) <> 1 OR new_revision <> old_max + 1 THEN RAISE EXCEPTION 'expected exactly one next revision after %, found otherwise', old_max; END IF;
  IF new_revision = old_revision OR (SELECT revision_id FROM graph_revision_current WHERE singleton) <> new_revision THEN RAISE EXCEPTION 'current graph pointer did not advance to new revision'; END IF;
  IF attached_catalog <> old_catalog OR (SELECT catalog_id FROM spatial_catalog_current WHERE singleton) <> old_catalog THEN RAISE EXCEPTION 'catalog changed during graph-only canary'; END IF;
  SELECT status, validation_result INTO catalog_status, catalog_validation FROM spatial_catalogs WHERE id = old_catalog;
  IF revision_status <> 'published' OR catalog_status <> 'published' OR NOT COALESCE((validation->>'valid')::boolean, false) OR NOT COALESCE((catalog_validation->>'valid')::boolean, false) THEN RAISE EXCEPTION 'published/valid status check failed'; END IF;
  SELECT count(*), count(*) FILTER (WHERE type='subreddit'), count(*) FILTER (WHERE type='user'), count(*) FILTER (WHERE type='post'), count(*) FILTER (WHERE type='comment') INTO nodes, subreddits, users, posts, comments FROM graph_revision_nodes WHERE revision_id = new_revision;
  SELECT count(*) INTO links FROM graph_revision_links WHERE revision_id = new_revision;
  SELECT count(*) INTO communities FROM graph_revision_communities WHERE revision_id = new_revision;
  SELECT count(*) INTO orphan_links FROM graph_revision_links l LEFT JOIN graph_revision_nodes s ON s.revision_id=l.revision_id AND s.id=l.source LEFT JOIN graph_revision_nodes t ON t.revision_id=l.revision_id AND t.id=l.target WHERE l.revision_id=new_revision AND (s.id IS NULL OR t.id IS NULL);
  SELECT count(*) INTO catalog_orphan_links FROM spatial_catalog_links l LEFT JOIN spatial_catalog_entities s ON s.catalog_id=l.catalog_id AND s.id=l.source LEFT JOIN spatial_catalog_entities t ON t.catalog_id=l.catalog_id AND t.id=l.target WHERE l.catalog_id=old_catalog AND (s.id IS NULL OR t.id IS NULL);
  IF nodes <> expected_nodes OR links <> expected_links OR communities <> expected_communities OR nodes <> (SELECT node_count FROM graph_revisions WHERE id=new_revision) OR links <> (SELECT link_count FROM graph_revisions WHERE id=new_revision) OR communities <> (SELECT community_count FROM graph_revisions WHERE id=new_revision) OR (current_setting('clustr.canary.node_cap')::bigint > 0 AND nodes > current_setting('clustr.canary.node_cap')::bigint) OR (current_setting('clustr.canary.link_cap')::bigint > 0 AND links > current_setting('clustr.canary.link_cap')::bigint) OR (current_setting('clustr.canary.subreddit_cap')::bigint > 0 AND subreddits > current_setting('clustr.canary.subreddit_cap')::bigint) OR (current_setting('clustr.canary.user_cap')::bigint > 0 AND users > current_setting('clustr.canary.user_cap')::bigint) OR (current_setting('clustr.canary.post_cap')::bigint > 0 AND posts > current_setting('clustr.canary.post_cap')::bigint) OR (current_setting('clustr.canary.comment_cap')::bigint > 0 AND comments > current_setting('clustr.canary.comment_cap')::bigint) OR orphan_links <> 0 OR catalog_orphan_links <> 0 THEN RAISE EXCEPTION 'counts, caps, or orphan links failed validation'; END IF;
  IF EXISTS (SELECT 1 FROM graph_revision_nodes WHERE revision_id=new_revision AND (x = 'NaN'::double precision OR y = 'NaN'::double precision OR z = 'NaN'::double precision OR x IN ('Infinity'::double precision, '-Infinity'::double precision) OR y IN ('Infinity'::double precision, '-Infinity'::double precision) OR z IN ('Infinity'::double precision, '-Infinity'::double precision))) THEN RAISE EXCEPTION 'non-finite graph node coordinate'; END IF;
  IF EXISTS (SELECT 1 FROM graph_revision_communities WHERE revision_id=new_revision AND (x = 'NaN'::double precision OR y = 'NaN'::double precision OR z = 'NaN'::double precision OR x IN ('Infinity'::double precision, '-Infinity'::double precision) OR y IN ('Infinity'::double precision, '-Infinity'::double precision) OR z IN ('Infinity'::double precision, '-Infinity'::double precision))) THEN RAISE EXCEPTION 'non-finite graph community coordinate'; END IF;
  IF EXISTS (SELECT 1 FROM graph_revision_bundles WHERE revision_id=new_revision AND (control_x = 'NaN'::double precision OR control_y = 'NaN'::double precision OR control_z = 'NaN'::double precision OR control_x IN ('Infinity'::double precision, '-Infinity'::double precision) OR control_y IN ('Infinity'::double precision, '-Infinity'::double precision) OR control_z IN ('Infinity'::double precision, '-Infinity'::double precision))) THEN RAISE EXCEPTION 'non-finite graph bundle coordinate'; END IF;
  IF EXISTS (SELECT 1 FROM graph_revisions WHERE id=new_revision AND (min_x IS NULL OR max_x IS NULL OR min_y IS NULL OR max_y IS NULL OR min_z IS NULL OR max_z IS NULL OR min_x = 'NaN'::double precision OR max_x = 'NaN'::double precision OR min_y = 'NaN'::double precision OR max_y = 'NaN'::double precision OR min_z = 'NaN'::double precision OR max_z = 'NaN'::double precision OR min_x IN ('Infinity'::double precision, '-Infinity'::double precision) OR max_x IN ('Infinity'::double precision, '-Infinity'::double precision) OR min_y IN ('Infinity'::double precision, '-Infinity'::double precision) OR max_y IN ('Infinity'::double precision, '-Infinity'::double precision) OR min_z IN ('Infinity'::double precision, '-Infinity'::double precision) OR max_z IN ('Infinity'::double precision, '-Infinity'::double precision))) THEN RAISE EXCEPTION 'non-finite graph bounds'; END IF;
  IF EXISTS (SELECT 1 FROM spatial_catalog_entities WHERE catalog_id=old_catalog AND (x = 'NaN'::double precision OR y = 'NaN'::double precision OR z = 'NaN'::double precision OR x IN ('Infinity'::double precision, '-Infinity'::double precision) OR y IN ('Infinity'::double precision, '-Infinity'::double precision) OR z IN ('Infinity'::double precision, '-Infinity'::double precision))) THEN RAISE EXCEPTION 'non-finite catalog coordinate'; END IF;
  IF EXISTS (SELECT 1 FROM spatial_catalogs WHERE id=old_catalog AND (min_x IS NULL OR max_x IS NULL OR min_y IS NULL OR max_y IS NULL OR min_z IS NULL OR max_z IS NULL OR min_x = 'NaN'::double precision OR max_x = 'NaN'::double precision OR min_y = 'NaN'::double precision OR max_y = 'NaN'::double precision OR min_z = 'NaN'::double precision OR max_z = 'NaN'::double precision OR min_x IN ('Infinity'::double precision, '-Infinity'::double precision) OR max_x IN ('Infinity'::double precision, '-Infinity'::double precision) OR min_y IN ('Infinity'::double precision, '-Infinity'::double precision) OR max_y IN ('Infinity'::double precision, '-Infinity'::double precision) OR min_z IN ('Infinity'::double precision, '-Infinity'::double precision) OR max_z IN ('Infinity'::double precision, '-Infinity'::double precision))) THEN RAISE EXCEPTION 'non-finite catalog bounds'; END IF;
END $$ \g /dev/null
SELECT gr.id AS revision_id, sc.id AS catalog_id, gr.node_count, gr.link_count, gr.community_count,
       gr.status AS revision_status, sc.status AS catalog_status,
       COALESCE((gr.validation_result->>'valid')::boolean, false) AS revision_valid,
       COALESCE((sc.validation_result->>'valid')::boolean, false) AS catalog_valid,
       (gr.spatial_catalog_id = scc.catalog_id) AS coherent
  FROM graph_revision_current grc
  JOIN graph_revisions gr ON gr.id = grc.revision_id
  CROSS JOIN spatial_catalog_current scc
  JOIN spatial_catalogs sc ON sc.id = scc.catalog_id
 WHERE grc.singleton AND scc.singleton;
SQL
[[ -s $EVIDENCE_FILE ]] || { printf 'canary validation evidence is absent or empty: %s\n' "$EVIDENCE_FILE" >&2; exit 1; }
```

Check `/ready` and revision-aware manifest/overview/region/community/entity/
search/detail/diff responses where locally reachable. Only then enable the
graph timer:

```bash
systemctl --user enable --now clustr-precalculate.timer
systemctl --user list-timers clustr-precalculate.timer
```

## Coherent pointer rollback

Before rollback, repeat this full two-manager quiescence check; stopping the
timer alone is insufficient, and a legacy wrapper can use a different lock.

```bash
set -euo pipefail
units=(clustr-precalculate.timer clustr-precalculate.service \
  clustr-spatial-catalog.service clustr-spatial-catalog-preflight.service)
quiesce_manager() {
  local manager=$1 unit load_state active_state
  shift
  local -a ctl=("$@")
  for unit in "${units[@]}"; do
    if ! load_state=$("${ctl[@]}" show --property=LoadState --value "$unit"); then
      printf '%s manager error reading LoadState for %s\n' "$manager" "$unit" >&2; return 1
    fi
    if [[ $load_state == not-found ]]; then
      printf '%s unit absent (LoadState=not-found): %s\n' "$manager" "$unit"
      continue
    fi
    if [[ $unit == clustr-precalculate.timer ]] && ! "${ctl[@]}" disable --now "$unit"; then
      printf '%s manager failed disabling %s\n' "$manager" "$unit" >&2; return 1
    fi
    if ! "${ctl[@]}" stop "$unit"; then
      printf '%s manager failed stopping %s\n' "$manager" "$unit" >&2; return 1
    fi
    if ! active_state=$("${ctl[@]}" show --property=ActiveState --value "$unit"); then
      printf '%s manager error reading ActiveState for %s\n' "$manager" "$unit" >&2; return 1
    fi
    case $active_state in
      inactive|failed) printf '%s unit stopped: %s (ActiveState=%s)\n' "$manager" "$unit" "$active_state" ;;
      *) printf '%s unit not quiescent: %s (ActiveState=%s)\n' "$manager" "$unit" "$active_state" >&2; return 1 ;;
    esac
  done
}
quiesce_manager user systemctl --user
quiesce_manager system sudo systemctl
if processes=$(pgrep -af 'clustr-precalculate|run-precalculation'); then
  printf '%s\n' "$processes" >&2
  printf '%s\n' 'Clustr worker/wrapper process remains; investigate and stop it.' >&2; exit 1
else
  pgrep_status=$?
  (( pgrep_status == 1 )) || { printf 'pgrep failed: %s\n' "$pgrep_status" >&2; exit 1; }
fi
if ! containers=$(docker ps -a --format '{{.Names}}'); then
  printf '%s\n' 'docker ps failed; cannot prove worker-container quiescence.' >&2; exit 1
fi
while IFS= read -r container; do
  if [[ $container == clustr-precalculate* ]]; then
    printf 'Clustr worker container remains: %s\n' "$container" >&2; exit 1
  fi
done <<<"$containers" # An empty inventory (no matching worker container) is required.
```

Choose a prior **published** graph revision; its referenced catalog must also be
published. Run the transaction on Almaz through the same strict SSH connection
used above (replace `REVISION_ID`). It validates the pair before changing either
pointer, updates `spatial_catalog_current` and `graph_revision_current`
atomically, and performs no cleanup:

```bash
ssh -F /dev/null -T -o BatchMode=yes -o ControlMaster=no -o ControlPath=none \
  -o ControlPersist=no -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile=/home/onnwee/.ssh/known_hosts 10.0.0.200 \
  "docker exec -i pg17-clustr bash -ceu 'exec psql -X -v ON_ERROR_STOP=1 -v revision_id=REVISION_ID -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\"'" <<'SQL'
BEGIN;
SELECT revision_id AS old_revision FROM graph_revision_current WHERE singleton;
SELECT catalog_id AS old_catalog FROM spatial_catalog_current WHERE singleton;
SELECT set_config('clustr.rollback_revision_id', :'revision_id', true);
DO $$
DECLARE chosen_revision bigint := current_setting('clustr.rollback_revision_id')::bigint;
DECLARE chosen_catalog bigint;
BEGIN
  SELECT gr.spatial_catalog_id INTO chosen_catalog
    FROM graph_revisions gr JOIN spatial_catalogs sc ON sc.id = gr.spatial_catalog_id
   WHERE gr.id = chosen_revision AND gr.status = 'published' AND sc.status = 'published';
  IF chosen_catalog IS NULL THEN
    RAISE EXCEPTION 'revision % is not published with a published catalog', chosen_revision;
  END IF;
  UPDATE spatial_catalog_current SET catalog_id = chosen_catalog WHERE singleton;
  IF NOT FOUND THEN RAISE EXCEPTION 'spatial_catalog_current singleton missing'; END IF;
  UPDATE graph_revision_current SET revision_id = chosen_revision WHERE singleton;
  IF NOT FOUND THEN RAISE EXCEPTION 'graph_revision_current singleton missing'; END IF;
END $$;
COMMIT;
SELECT grc.revision_id, scc.catalog_id, gr.status AS revision_status,
       sc.status AS catalog_status, gr.spatial_catalog_id = scc.catalog_id AS coherent
  FROM graph_revision_current grc CROSS JOIN spatial_catalog_current scc
  JOIN graph_revisions gr ON gr.id = grc.revision_id
  JOIN spatial_catalogs sc ON sc.id = scc.catalog_id
 WHERE grc.singleton AND scc.singleton;
SQL
```

Require one row with both statuses `published` and `coherent = t`; then check
`/ready` and the revision-aware read routes. Record old/new IDs and results.
Never automatically delete failed/staging revisions, catalogs, or source data.
