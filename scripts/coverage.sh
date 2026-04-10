#!/bin/bash
set -e

# Test coverage script for My Patreon Manager
# Runs all tests with coverage and generates reports

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERAGE_DIR="${PROJECT_ROOT}/coverage"
COVERAGE_FILE="${COVERAGE_DIR}/coverage.out"
COVERAGE_HTML="${COVERAGE_DIR}/html"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
ARCHIVE_DIR="${PROJECT_ROOT}/test-results/archive/${TIMESTAMP}"

echo "📊 Running full test suite with coverage..."

# Create directories
mkdir -p "${COVERAGE_DIR}"
mkdir -p "${COVERAGE_HTML}"
mkdir -p "${ARCHIVE_DIR}"

# First, run all tests to ensure they pass
echo "🧪 Running all test suites..."
go test ./internal/... ./cmd/... ./tests/... -v -count=1

# Run tests with coverage for production code (internal and cmd packages)
# Run tests on all packages to collect coverage
echo "📈 Collecting coverage data..."
go test -coverprofile="${COVERAGE_FILE}" -covermode=atomic -coverpkg=./internal/...,./cmd/... ./... -count=1

# Generate HTML report
echo "📊 Generating HTML coverage report..."
go tool cover -html="${COVERAGE_FILE}" -o "${COVERAGE_HTML}/index.html"

# Generate summary
echo "📋 Coverage summary:"
go tool cover -func="${COVERAGE_FILE}"

# Check if coverage is 100% per package (required) for internal and cmd packages
echo "📊 Checking per-package coverage..."
ALL_PASS=true

# Use go tool cover -func to get per-file coverage, aggregate by package
go tool cover -func="${COVERAGE_FILE}" | grep -E "^github.com/milos85vasic/My-Patreon-Manager/(internal|cmd)" > "${COVERAGE_DIR}/per-package.txt" || true

# Parse and compute average per package
declare -A PACKAGE_SUM
declare -A PACKAGE_COUNT
while IFS= read -r line; do
    # Extract file path and percentage
    # Format: github.com/.../file.go:line: funcName percentage%
    # We'll split by tabs
    perc=$(echo "$line" | awk -F'\t' '{print $NF}' | sed 's/%//')
    file=$(echo "$line" | awk -F'\t' '{print $1}' | sed 's/:[0-9]*$//')
    # Extract package path (remove filename)
    package=$(dirname "$file")
    # Strip prefix
    package=${package#github.com/milos85vasic/My-Patreon-Manager/}
    # Skip if package is not internal or cmd (should be filtered already)
    if [[ "$package" != internal* && "$package" != cmd* ]]; then
        continue
    fi
    PACKAGE_SUM["$package"]=$(bc -l <<< "${PACKAGE_SUM[$package]:-0} + $perc" 2>/dev/null || echo "${PACKAGE_SUM[$package]:-0}")
    PACKAGE_COUNT["$package"]=$(( ${PACKAGE_COUNT[$package]:-0} + 1 ))
done < "${COVERAGE_DIR}/per-package.txt"

# Print per-package coverage and check for < 100%
echo "Per-package coverage summary:"
for pkg in $(echo "${!PACKAGE_SUM[@]}" | tr ' ' '\n' | sort); do
    sum=${PACKAGE_SUM[$pkg]}
    count=${PACKAGE_COUNT[$pkg]}
    avg=$(bc -l <<< "scale=1; $sum / $count" 2>/dev/null || echo "0")
    # Convert to float comparison
    if (( $(echo "$avg < 100" | bc -l 2>/dev/null || echo "1") )); then
        echo "❌ $pkg: ${avg}%"
        ALL_PASS=false
    else
        echo "✅ $pkg: ${avg}%"
    fi
done

if [[ "$ALL_PASS" != "true" ]]; then
    echo "❌ Some packages have coverage below 100%. Failing."
    exit 1
fi

# Also check total coverage as summary
TOTAL_COVERAGE=$(go tool cover -func="${COVERAGE_FILE}" | grep total | awk '{print $3}' | sed 's/%//')
echo "✅ Total coverage: ${TOTAL_COVERAGE}%"

if (( $(echo "$TOTAL_COVERAGE < 100" | bc -l) )); then
    echo "❌ Total coverage is below 100%. Failing."
    exit 1
fi

# Archive results
cp "${COVERAGE_FILE}" "${ARCHIVE_DIR}/"
cp -r "${COVERAGE_HTML}" "${ARCHIVE_DIR}/"
echo "📦 Results archived to ${ARCHIVE_DIR}"

# Generate JSON report for CI (optional)
echo "{\"coverage\": ${TOTAL_COVERAGE}, \"timestamp\": \"${TIMESTAMP}\"}" > "${COVERAGE_DIR}/coverage-summary.json"

echo "🎉 Coverage script completed successfully!"
echo "📁 HTML report: file://${COVERAGE_HTML}/index.html"