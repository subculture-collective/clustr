# Production database clone: Almaz to Kvant

This runbook creates an encrypted, snapshot-consistent clone of Almaz's Clustr
database on Kvant. It does not stop PostgreSQL, pause the crawler, alter source
rows, or overwrite an existing clone. Export execution and cleanup are explicit
production actions; `--preflight` is read-only.

## Data flow

```mermaid
flowchart LR
  P["Almaz PostgreSQL 17"] -->|"exported read-only snapshot"| D["custom pg_dump"]
  D --> V["pg_restore list validation"]
  V --> E["GPG encryption with Kvant public key"]
  E --> C["SHA-256 manifest"]
  C -->|"resumable rsync over SSH"| K["Kvant incoming archive"]
  K --> R["new digest-pinned PostgreSQL clone"]
  R --> Q["count and schema identity verification"]
  Q --> W["calculation and publication rehearsal"]
```

## One-time Kvant setup

1. Install the exact release checkout and systemd template on Kvant. Installation
   creates durable storage roots, a Kvant-only encryption key, a mode-`0640`
   environment file, and reloads systemd. It does not start an export:

   ```bash
   sudo ./deploy/kvant/install-clone-service.sh "$(pwd)"
   systemctl cat clustr-clone-export@.service
   ```

2. Back up `/etc/clustr/clone-gpg` separately from database exports. The private
   key never leaves Kvant. Re-running the installer preserves an existing key
   and environment file.
3. Review `/etc/clustr/clustr-clone.env` against
   [clustr-clone.env.example](../../deploy/clone/clustr-clone.env.example). It
   contains no database password or private key.
4. Verify `ssh -o BatchMode=yes almaz true`. Almaz's PostgreSQL remains private;
   the source helper executes through SSH and uses credentials already scoped to
   the database container.

## Read-only preflight

```bash
/opt/clustr/scripts/clone/export-to-kvant.sh \
  /etc/clustr/clustr-clone.env --preflight
```

Require all of these before export approval:

- exact source host, container, database, PostgreSQL 17 version, and byte size;
- at least 1.25 times the database size free in Almaz's export parent;
- at least 2.5 times the database size free in Kvant's clone parent;
- the configured Kvant port is reserved for the new clone;
- no old export is holding a snapshot transaction.

Preflight must not create the source or destination roots.

## Execute and monitor

Run from Kvant after explicit production-export approval. The systemd instance
keeps the clone operation and its result associated with one immutable clone ID:

```bash
clone_id="clustr-$(date -u +%Y%m%dT%H%M%SZ)-$(openssl rand -hex 4)"
sudo systemctl start "clustr-clone-export@${clone_id}.service"
sudo journalctl -fu "clustr-clone-export@${clone_id}.service"
```

The source Module permits only one export. During `pg_dump`, monitor Almaz's
free space, PostgreSQL transaction age, load, and container health. Abort if it
threatens production. An abort keeps production unchanged, removes any plaintext
partial archive, and marks the export `FAILED`.

Restarting the same systemd instance is idempotent after a completed source
export: it checksum-validates and resumes transfer/restore without taking a new
production snapshot. A source export that never reached `READY` is not reused.

Success requires:

- source `READY`, valid checksums, and an encrypted `.dump.gpg` only;
- Kvant `READY`, no retained plaintext dump, loopback-only PostgreSQL listener,
  container health `healthy`, and restart policy `unless-stopped`;
- `verification.json` status `verified` with exact source/clone counts for
  subreddits, users, posts, and comments plus matching migration, encoding,
  collation, and extensions;
- the manifest's image is pinned by digest;
- the source production API/crawler remain healthy.

After a Kvant reboot, Docker restores the verified clone automatically from its
bind-mounted `/mnt/data2/clustr-clones/<clone-id>/pgdata`. Confirm both the
container and application contract rather than relying on process state alone:

```bash
docker inspect "${clone_id}-pg17" \
  --format 'restart={{.HostConfig.RestartPolicy.Name}} health={{.State.Health.Status}}'
jq '{status,runtime,expected:.expected.tables,actual:.actual.tables}' \
  "/mnt/data2/clustr-clones/${clone_id}/verification.json"
```

## Resume and failure recovery

An interrupted export itself is restarted under a new clone identity. An
interrupted transfer is resumable because the immutable source export remains:

```bash
/opt/clustr/scripts/clone/export-to-kvant.sh \
  /etc/clustr/clustr-clone.env --resume "$clone_id"
```

Restore is intentionally not resumed inside a partly populated database. Inspect
the `FAILED` marker and logs, remove that exact failed clone with confirmation,
then run `--resume`; it reuses the checksum-verified encrypted export and creates
a clean destination.

```bash
/opt/clustr/scripts/clone/remove-clone.sh \
  /mnt/data2/clustr-clones "$clone_id" "--confirm-${clone_id}"
```

## Handoff to calculation

Read the clone's `READY` file for its loopback port and use its private password
file to construct `DATABASE_URL` without printing it. Apply migrations to the
clone, run the full catalog/revision calculation, and retain calculation evidence
under the clone identity. Never point production readers at the clone.

The installed `clustr-clone-calculate@<clone-id>.service` owns this rehearsal.
It validates clone identity, health, loopback binding, and the pinned worker
image before starting the full catalog/revision calculation with bounded CPU
and memory. The database credential is composed from the protected clone
password through an ephemeral file descriptor rather than stored in another
environment file.
On Kvant, the supported calculation profile gives PostgreSQL 16 CPUs and a
an enlarged shared-memory mount, and sets `SPATIAL_CATALOG_PARALLEL_WORKERS=12` for
large catalog queries. Keep the application default at zero on hosts that have
not explicitly enlarged the database container's `/dev/shm`.
After a catalog-only failure, an operator may use `--publish-existing` to reuse
an already completed graph workspace. This mode still rebuilds and validates
the complete catalog and immutable revision; it only avoids repeating source
projection, hierarchy, and layout.

## Retention and explicit cleanup

Keep the encrypted source and incoming exports until clone verification and the
calculation rehearsal both succeed. Keep the latest verified clone through the
Almaz cutover rollback window. Then remove artifacts independently:

```bash
/opt/clustr/scripts/clone/export-to-kvant.sh \
  /etc/clustr/clustr-clone.env --cleanup-source "$clone_id" "--confirm-${clone_id}"

/opt/clustr/scripts/clone/remove-incoming-export.sh \
  /mnt/data2/clustr-exports "$clone_id" "--confirm-${clone_id}"

/opt/clustr/scripts/clone/remove-clone.sh \
  /mnt/data2/clustr-clones "$clone_id" "--confirm-${clone_id}"
```

There is no automatic retention deletion. This is deliberate: clone identities
are release evidence and cleanup should happen only after the operator verifies
which revision and backup window they support.

## Security and recovery notes

- SSH encrypts transport; GPG encrypts the archive at rest on both hosts.
- Only Kvant stores the private key. Almaz receives an ephemeral public keyring.
- Database passwords never enter the export manifest, command line, or transfer.
- Clone PostgreSQL binds only to `127.0.0.1` and uses a generated clone-only
  credential.
- SHA-256 and GPG's protected ciphertext detect corruption; only Kvant's private
  key can decrypt the recipient-bound archive.
- This logical clone excludes cluster roles/ACLs and is not a substitute for the
  independent production backup required before Almaz migration or cutover.
