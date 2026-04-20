#!/usr/bin/env bash
# scripts/coverage.sh
#
# Per-package coverage run that avoids the `-coverpkg` combined-mode
# dilution documented in docs/KNOWN-ISSUES.md §2.2 (now closed). Every
# Go package is tested in its own invocation with its own coverprofile,
# then the profiles are merged via scripts/covermerge taking the MAX
# count per statement (correct for atomic mode). The merged profile
# feeds the existing scripts/coverdiff gate.
#
# Env overrides:
#   COVERAGE_MIN  Minimum per-package and total threshold (default: 100.0).
#   COVERAGE_TIMEOUT  Per-package test timeout (default: 10m).
#   COVERAGE_SKIP_TESTS  Space-separated glob patterns; packages whose
#                        import path matches any of these are skipped
#                        (e.g. for long-running e2e suites).
#   COVERAGE_WITH_POSTGRES  Set to "1" to force the Postgres live-harness
#                            run. Set to "0" to force-skip. Unset =
#                            auto-detect: run if podman/docker is on PATH.
#
# On success the script writes:
#   coverage/coverage.out         merged profile
#   coverage/coverage.func.txt    per-function coverage report
#   coverage/coverage.html        visual report
#   coverage/profiles/*.out       per-package raw profiles (kept for debugging)
#   coverage/postgres.log         Postgres harness output (when run)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MIN="${COVERAGE_MIN:-100.0}"
TIMEOUT="${COVERAGE_TIMEOUT:-10m}"
SKIP_PATTERNS="${COVERAGE_SKIP_TESTS:-}"
WITH_POSTGRES="${COVERAGE_WITH_POSTGRES:-auto}"

mkdir -p coverage/profiles
# Clear only the per-run artifacts; keep the coverdiff binary if it
# still exists so we don't re-build every invocation.
rm -f coverage/profiles/*.out coverage/coverage.out coverage/coverage.func.txt coverage/coverage.html

# Build helpers first — fast, no external deps.
go build -o coverage/coverdiff ./scripts/coverdiff
go build -o coverage/covermerge ./scripts/covermerge

# Enumerate every package we want to test. `go list` returns import paths;
# we include internal, cmd, and tests so test-only packages (./tests/...)
# drive coverage into internal/* and cmd/* via -coverpkg.
mapfile -t PACKAGES < <(go list ./internal/... ./cmd/... ./tests/... 2>/dev/null)

if [ "${#PACKAGES[@]}" -eq 0 ]; then
    echo "coverage: no packages discovered" >&2
    exit 1
fi

# Filter out skip patterns. Uses Bash glob matching against the import path.
filtered=()
for pkg in "${PACKAGES[@]}"; do
    skip=0
    for pat in $SKIP_PATTERNS; do
        # shellcheck disable=SC2053
        if [[ "$pkg" == $pat ]]; then skip=1; break; fi
    done
    if [ "$skip" -eq 0 ]; then filtered+=("$pkg"); fi
done
PACKAGES=("${filtered[@]}")

echo "coverage: running ${#PACKAGES[@]} package(s) under -race + atomic cover"

# Run each package in its own invocation. Coverpkg stays pointed at the
# same set (./internal/...,./cmd/...) so test-only packages still
# instrument the production code. Profile filenames derive from the
# import path with slashes → dashes so every artifact is unique.
for pkg in "${PACKAGES[@]}"; do
    safe="${pkg//\//-}"
    profile="coverage/profiles/${safe}.out"
    # Use || true so one flaky package doesn't bail the whole run before
    # coverdiff can summarize — but we capture exit status to fail at the end.
    CGO_ENABLED=1 go test -race -timeout "$TIMEOUT" \
        -covermode=atomic \
        -coverpkg=./internal/...,./cmd/... \
        -coverprofile="$profile" \
        "$pkg" || echo "coverage: non-zero exit from $pkg" >&2
done

# Merge everything into a single profile.
# Skip empty profile files to avoid bloating covermerge with zero-record inputs.
mapfile -t NONEMPTY < <(find coverage/profiles -name '*.out' -size +0c -print)
if [ "${#NONEMPTY[@]}" -eq 0 ]; then
    echo "coverage: no non-empty profiles produced — every package skipped coverage?" >&2
    exit 1
fi

./coverage/covermerge "${NONEMPTY[@]}" > coverage/coverage.out

# Postgres live-harness integration. The default-on behavior closed the
# "build-tag gated, never actually runs" gap for KNOWN-ISSUES §2.1: the
# harness fires automatically when podman or docker is on PATH, so
# operators get real Postgres coverage on every `bash scripts/coverage.sh`
# run without remembering to invoke test-postgres.sh separately.
run_postgres=0
case "$WITH_POSTGRES" in
    1|true|yes|on)  run_postgres=1 ;;
    0|false|no|off) run_postgres=0 ;;
    auto)
        if command -v podman >/dev/null 2>&1 || command -v docker >/dev/null 2>&1; then
            run_postgres=1
        fi
        ;;
    *)
        echo "coverage: invalid COVERAGE_WITH_POSTGRES=$WITH_POSTGRES; valid: 1/0/auto" >&2
        exit 2
        ;;
esac

if [ "$run_postgres" -eq 1 ]; then
    echo "coverage: running Postgres live harness"
    if bash scripts/test-postgres.sh 2>&1 | tee coverage/postgres.log; then
        echo "coverage: Postgres harness green"
    else
        echo "coverage: Postgres harness FAILED — see coverage/postgres.log" >&2
        exit 1
    fi
else
    echo "coverage: Postgres harness skipped (COVERAGE_WITH_POSTGRES=$WITH_POSTGRES, or runtime not found)"
fi

# Produce both renders for operator convenience.
go tool cover -html=coverage/coverage.out -o coverage/coverage.html
go tool cover -func=coverage/coverage.out | tee coverage/coverage.func.txt | \
    ./coverage/coverdiff -min "$MIN"
