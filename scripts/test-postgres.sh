#!/usr/bin/env bash
# scripts/test-postgres.sh
#
# Runs the live Postgres integration harness (tests/integration/postgres_live_test.go)
# against a throwaway Postgres instance.  Closes KNOWN-ISSUES §2.1.
#
# Usage:
#   bash scripts/test-postgres.sh            # use podman (default)
#   RUNTIME=docker bash scripts/test-postgres.sh   # or docker
#
# The helper:
#   1. Starts a postgres:16-alpine container on a throwaway port.
#   2. Waits for `pg_isready` to confirm readiness.
#   3. Exports POSTGRES_TEST_DSN.
#   4. Runs `go test -tags postgres -race ./tests/integration/...`.
#   5. Stops and removes the container on exit (even on failure).
#
# Env overrides:
#   RUNTIME       Container runtime (default: podman; supports docker).
#   PG_IMAGE      Postgres image (default: postgres:16-alpine).
#   PG_PORT       Host port (default: a random port in 15432-15499).
#   PG_DB         Database name (default: patreon_manager_test).
#   PG_USER       Postgres user (default: patreon).
#   PG_PASSWORD   Postgres password (default: patreontest).
#   CONTAINER_NAME  Container name (default: patreon-test-pg-<pid>).
#   GO_TEST_ARGS  Extra args forwarded to go test (e.g. `-run TestPostgres_`).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

RUNTIME="${RUNTIME:-podman}"
PG_IMAGE="${PG_IMAGE:-postgres:16-alpine}"
# Random port in [15432, 15499] keeps parallel invocations from colliding.
PG_PORT="${PG_PORT:-$(( RANDOM % 68 + 15432 ))}"
PG_DB="${PG_DB:-patreon_manager_test}"
PG_USER="${PG_USER:-patreon}"
PG_PASSWORD="${PG_PASSWORD:-patreontest}"
CONTAINER_NAME="${CONTAINER_NAME:-patreon-test-pg-$$}"
GO_TEST_ARGS="${GO_TEST_ARGS:-}"

if ! command -v "$RUNTIME" >/dev/null 2>&1; then
    echo "test-postgres: '$RUNTIME' is not on PATH; install podman/docker or override RUNTIME" >&2
    exit 1
fi

cleanup() {
    echo "test-postgres: stopping container $CONTAINER_NAME"
    "$RUNTIME" rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "test-postgres: starting $PG_IMAGE as $CONTAINER_NAME on port $PG_PORT"
"$RUNTIME" run -d --rm --name "$CONTAINER_NAME" \
    -e POSTGRES_DB="$PG_DB" \
    -e POSTGRES_USER="$PG_USER" \
    -e POSTGRES_PASSWORD="$PG_PASSWORD" \
    -p "$PG_PORT":5432 \
    "$PG_IMAGE" >/dev/null

# Poll pg_isready inside the container. Give it 30s max; the image
# boots in ~2s locally but we want headroom on slower hosts.
for attempt in $(seq 1 30); do
    if "$RUNTIME" exec "$CONTAINER_NAME" pg_isready -U "$PG_USER" -d "$PG_DB" >/dev/null 2>&1; then
        echo "test-postgres: ready after ${attempt}s"
        break
    fi
    sleep 1
    if [ "$attempt" -eq 30 ]; then
        echo "test-postgres: container did not become ready within 30s" >&2
        "$RUNTIME" logs "$CONTAINER_NAME" >&2 || true
        exit 1
    fi
done

export POSTGRES_TEST_DSN="host=localhost port=$PG_PORT user=$PG_USER password=$PG_PASSWORD dbname=$PG_DB sslmode=disable"
echo "test-postgres: POSTGRES_TEST_DSN=${POSTGRES_TEST_DSN}"

CGO_ENABLED=1 go test -tags postgres -race -timeout 3m ${GO_TEST_ARGS} ./tests/integration/...
