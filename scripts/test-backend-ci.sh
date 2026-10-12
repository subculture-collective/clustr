#!/usr/bin/env bash
# A fresh migrated database belongs only to this invocation.
set -euo pipefail
cd "$(dirname "$0")/../backend"

postgres_container=$(docker run -d --memory=512m --cpus=1 \
  --network bridge -p 127.0.0.1::5432 \
  -e POSTGRES_USER=clustr_test -e POSTGRES_PASSWORD=test-only-password \
  -e POSTGRES_DB=clustr_test postgres:17-alpine)
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'docker rm --force --volumes "$postgres_container" >/dev/null 2>&1 || true' EXIT

ready=false
for attempt in {1..90}; do
  if [[ "$(docker inspect --format '{{.State.Running}}' "$postgres_container")" != true ]]; then
    break
  fi
  if docker exec "$postgres_container" pg_isready -h 127.0.0.1 -U clustr_test -d clustr_test >/dev/null; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  echo "Disposable PostgreSQL failed readiness (probe $attempt)." >&2
  docker inspect --format 'status={{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}}' "$postgres_container" >&2 || true
  docker logs --tail 40 "$postgres_container" >&2 || true
  exit 1
fi

if [[ -f /.dockerenv ]]; then
  postgres_address=$(docker inspect --format '{{(index .NetworkSettings.Networks "bridge").IPAddress}}' "$postgres_container")
  postgres_port=5432
else
  postgres_address=127.0.0.1
  postgres_port=$(docker port "$postgres_container" 5432/tcp | cut -d: -f2)
fi
export DATABASE_URL="postgres://clustr_test:test-only-password@${postgres_address}:${postgres_port}/clustr_test?sslmode=disable"
export TEST_DATABASE_URL="$DATABASE_URL"
export MIGRATIONS_DIR="$PWD/migrations"
go run ./cmd/migrate
# Packages share destructive graph fixtures; serialize them on the owned DB.
go test -count=1 -p 1 -timeout 10m -v -race -coverprofile=coverage.out ./...
