#!/usr/bin/env bash
# scripts/fuzz.sh
#
# Runs every Fuzz target in tests/fuzz/ for a bounded wall-clock
# budget per target. Meant for CI / nightly / pre-release runs —
# the seed-only invocation that fires under `go test ./...` is kept
# as a regression smoke; this script drives real coverage-guided
# exploration.
#
# Env overrides:
#   FUZZTIME    Wall time per target (default: 30s). Use "10m" for
#               nightly, "5m" for operator-local.
#   FUZZ_PKG    Target package (default: ./tests/fuzz/).
#   FUZZ_FILTER Glob passed to -run for the test selector so a
#               failed run can be repro'd against a single target.
#
# Output:
#   coverage/fuzz/<target>.log     per-target stdout
#   testdata/fuzz/<target>/corpus  updated seed corpus (committed by operator)
#
# Behaviour: if any target finds a crash, the crash input is persisted
# under `testdata/fuzz/<target>/` by the Go fuzz engine; operators
# add a seed for it and write a fix. This script exits non-zero on
# the first failing target.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

FUZZTIME="${FUZZTIME:-30s}"
FUZZ_PKG="${FUZZ_PKG:-./tests/fuzz/}"
FUZZ_FILTER="${FUZZ_FILTER:-}"

mkdir -p coverage/fuzz

# Enumerate Fuzz targets by parsing the package's _test.go files.
mapfile -t TARGETS < <(grep -rh "^func Fuzz" "$ROOT/tests/fuzz/" | awk '{print $2}' | sed 's/(.*//')

if [ "${#TARGETS[@]}" -eq 0 ]; then
    echo "fuzz: no Fuzz* targets found in $FUZZ_PKG" >&2
    exit 1
fi

echo "fuzz: running ${#TARGETS[@]} target(s) for $FUZZTIME each"

for target in "${TARGETS[@]}"; do
    if [ -n "$FUZZ_FILTER" ] && [[ "$target" != $FUZZ_FILTER ]]; then
        continue
    fi
    echo "fuzz: $target"
    log="coverage/fuzz/${target}.log"
    CGO_ENABLED=1 go test -run '^$' -fuzz="^${target}\$" -fuzztime="$FUZZTIME" "$FUZZ_PKG" 2>&1 | tee "$log"
done

echo "fuzz: done"
