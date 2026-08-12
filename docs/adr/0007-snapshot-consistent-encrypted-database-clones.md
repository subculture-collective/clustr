# ADR 0007: Snapshot-consistent encrypted database clones

Status: accepted

## Context

The graph rehearsal originally consumed an operator-provided dump file. It did
not define how that file was created, prove that source counts belonged to the
dump snapshot, encrypt the archive at rest, resume an interrupted transfer, or
prevent a restore from replacing an existing Kvant clone. Physical copies of
Almaz's PostgreSQL data directory would couple recovery to the live filesystem,
WAL state, ownership, and exact server lifecycle.

## Decision

Clustr uses a logical clone pipeline with the following deep Module boundary:

- the source export Implementation holds a read-only repeatable-read transaction,
  exports its PostgreSQL snapshot, and gives that same snapshot to `pg_dump` and
  the source manifest query;
- the archive uses PostgreSQL custom format, no owners or ACLs, and is validated
  with `pg_restore --list` before encryption;
- Almaz imports only a temporary Kvant public GPG key. The plaintext dump is
  deleted before the export receives `READY`;
- SHA-256 covers the encrypted archive, source metadata, and manifest;
- `rsync --partial --append-verify` is the resumable transport over authenticated
  SSH;
- Kvant restores into a new clone identity, digest-pinned PostgreSQL image,
  unique bind-mounted data directory, loopback-only port, and generated clone-only
  credential;
- verification compares source and clone table counts, migration identity,
  encoding, collation, and extensions before the clone receives `READY`;
- cleanup is an explicit, separately confirmed operation. A failed export or
  clone remains inspectable and cannot be silently reused.

The archive intentionally excludes PostgreSQL roles, ownership, ACLs, and host
configuration. The clone is an application-data/calculation environment, not a
drop-in disaster-recovery replacement for the whole cluster.

## Consequences

The Interface is slower than a filesystem snapshot but portable, least-privilege,
auditable, and independent of production storage internals. A held exported
snapshot can delay vacuum cleanup during a long dump, so only one export may run
and operators must monitor duration and Almaz disk/database pressure. The source
and incoming encrypted exports are retained until verification, then removed
explicitly. The private Kvant key must be backed up separately; losing it makes
retained exports intentionally unrecoverable.

## Rejected alternatives

- `rsync` of `PGDATA`: unsafe without a coordinated physical-backup/WAL contract.
- one streaming pipe: encrypted but not resumable after connection loss.
- an unencrypted custom dump: simpler, but leaves production data readable at
  rest on both hosts.
- calculating through a long-lived database tunnel only: avoids a clone but
  makes the heavy read workload and launch rehearsal inseparable from production.
