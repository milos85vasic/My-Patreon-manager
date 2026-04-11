#!/usr/bin/env bash
set -euo pipefail
# smoke test: script runs, produces coverage/coverage.out, exits nonzero on <100
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
cp -r . "$tmpdir/repo"
cd "$tmpdir/repo"
# Should fail loudly because coverage is currently < 100
if bash scripts/coverage.sh > "$tmpdir/out" 2>&1; then
  echo "expected coverage.sh to fail on current 82.7% coverage"
  cat "$tmpdir/out"
  exit 1
fi
grep -q "packages below threshold" "$tmpdir/out" || { echo "missing enforcement message"; cat "$tmpdir/out"; exit 1; }
echo "coverage_test.sh: PASS"
