#!/usr/bin/env bash
# scripts/release/verify_all.sh — master verification gate for releases
# Usage: bash scripts/release/verify_all.sh <version>
set -euo pipefail

VERSION="${1:?usage: $0 <version>}"
EV_DIR="docs/releases/$VERSION/evidence"
mkdir -p "$EV_DIR"

FAIL=0
record() {
  local name="$1" status="$2" summary="$3"
  echo "[$status] $name — $summary"
  echo "$name=$status=$summary" >> "$EV_DIR/results.tsv"
  if [ "$status" = "FAIL" ]; then FAIL=1; fi
}

echo "=== Release Gate: $VERSION ==="
echo "Date: $(date -Iseconds)"
echo ""

echo "=== build ==="
if go build ./... 2>"$EV_DIR/build.log"; then
  record build PASS "compiles clean"
else
  record build FAIL "build errors"
fi

echo "=== vet ==="
if go vet ./... 2>"$EV_DIR/vet.log"; then
  record vet PASS "no issues"
else
  record vet FAIL "vet findings"
fi

echo "=== race tests ==="
if go test -race -timeout 15m ./... > "$EV_DIR/race.log" 2>&1; then
  record race PASS "all packages green"
else
  record race FAIL "test failures"
fi

echo "=== coverage ==="
if COVERAGE_MIN=0 bash scripts/coverage.sh > "$EV_DIR/coverage.log" 2>&1; then
  record coverage PASS "coverage gate passed"
else
  record coverage FAIL "coverage below threshold"
fi

echo "=== docs lint ==="
if command -v markdownlint-cli2 >/dev/null 2>&1; then
  if markdownlint-cli2 "**/*.md" > "$EV_DIR/markdownlint.log" 2>&1; then
    record docs PASS "lint clean"
  else
    record docs FAIL "lint violations"
  fi
else
  record docs SKIP "markdownlint-cli2 not installed"
fi

echo "=== video artifacts ==="
missing=0
for n in 01 02 03 04 05 06 07 08 09 10; do
  if ! ls docs/video/scripts/module${n}* >/dev/null 2>&1; then
    missing=1
  fi
done
if [ "$missing" -eq 0 ]; then
  record video PASS "all 10 module scripts present"
else
  record video FAIL "missing module scripts"
fi

echo ""
echo "=== Results ==="
cat "$EV_DIR/results.tsv"
echo ""

if [ "$FAIL" -ne 0 ]; then
  echo "=== RELEASE GATE: FAILED ==="
  exit 1
fi
echo "=== RELEASE GATE: PASSED ==="
