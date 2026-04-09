#!/usr/bin/env bash
set -euo pipefail

echo "=== Running test coverage ==="

COVERAGE_DIR="coverage"
mkdir -p "$COVERAGE_DIR"

COVERPROFILE="$COVERAGE_DIR/coverage.out"
HTML_REPORT="$COVERAGE_DIR/coverage.html"

go test -coverprofile="$COVERPROFILE" -covermode=atomic ./...

echo ""
echo "=== Coverage Summary ==="
go tool cover -func="$COVERPROFILE" | tail -1

echo ""
echo "=== Generating HTML report ==="
go tool cover -html="$COVERPROFILE" -o "$HTML_REPORT"

echo ""
echo "HTML report: $HTML_REPORT"
echo "Done."
