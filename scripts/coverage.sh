#!/usr/bin/env bash
# scripts/coverage.sh
# Runs the full test suite under -race with coverage, then enforces 100% per-package
# and 100% total via scripts/coverdiff.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p coverage
OUT="coverage/coverage.out"
MIN="${COVERAGE_MIN:-100.0}"

# Build coverdiff helper (fast; no external deps).
go build -o coverage/coverdiff ./scripts/coverdiff

# Run full test matrix with race detector + coverage across internal/ and cmd/.
CGO_ENABLED=1 go test -race -timeout 10m \
  -covermode=atomic \
  -coverpkg=./internal/...,./cmd/... \
  -coverprofile="$OUT" \
  ./internal/... ./cmd/... ./tests/...

go tool cover -html="$OUT" -o coverage/coverage.html

# Enforce.
go tool cover -func="$OUT" | tee coverage/coverage.func.txt | \
  ./coverage/coverdiff -min "$MIN"
