#!/usr/bin/env bash
# scripts/bench.sh
#
# Runs every Benchmark* in tests/benchmark/ with -benchmem and a
# moderate benchtime (1s per benchmark). Output written to
# coverage/bench.txt so operators can diff runs over time and catch
# regressions. Exits non-zero if any benchmark panics or the go
# toolchain rejects the invocation.
#
# Env overrides:
#   BENCHTIME   Per-benchmark wall time (default: 1s).
#   BENCH_RUN   Filter passed as `-bench=`; default is ALL ("."), set to
#               a pattern (e.g. "BenchmarkRepoignore") to run a subset.
#   BENCH_PKG   Target package (default: ./tests/benchmark/).
#   BENCH_CPU   Comma-separated -cpu list (default: unset, uses host).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BENCHTIME="${BENCHTIME:-1s}"
BENCH_RUN="${BENCH_RUN:-.}"
BENCH_PKG="${BENCH_PKG:-./tests/benchmark/}"
mkdir -p coverage

cpu_flag=""
if [ -n "${BENCH_CPU:-}" ]; then
    cpu_flag="-cpu=${BENCH_CPU}"
fi

echo "bench: running -bench=$BENCH_RUN -benchtime=$BENCHTIME on $BENCH_PKG"
CGO_ENABLED=1 go test \
    -run '^$' \
    -bench="$BENCH_RUN" \
    -benchmem \
    -benchtime="$BENCHTIME" \
    $cpu_flag \
    "$BENCH_PKG" | tee coverage/bench.txt
