#!/usr/bin/env bash
# scripts/security/run_all.sh — orchestrates all security scanners
# Usage: bash scripts/security/run_all.sh
# Requires: podman-compose, SNYK_TOKEN (optional), SONAR_TOKEN (optional)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

mkdir -p coverage docs/security/baselines

FAIL=0
run_scanner() {
  local name="$1"
  echo "== $name =="
  if podman-compose -f docker-compose.security.yml run --rm "$name" 2>&1; then
    echo "  ✓ $name passed"
  else
    echo "  ✗ $name failed (findings or error)"
    FAIL=1
  fi
}

echo "=== Security Scan Suite ==="
echo "Date: $(date -Iseconds)"
echo ""

# One-shot scanners
run_scanner gosec
run_scanner govulncheck
run_scanner gitleaks
run_scanner trivy-fs
run_scanner semgrep
run_scanner syft

# Snyk (requires token)
if [ -n "${SNYK_TOKEN:-}" ]; then
  run_scanner snyk
else
  echo "== snyk =="
  echo "  ⊘ SNYK_TOKEN unset, skipping"
fi

# SonarQube (requires running instance + token)
if [ -n "${SONAR_TOKEN:-}" ]; then
  echo "== sonarqube =="
  echo "  Starting SonarQube (may take 2-5 min on first run)..."
  podman-compose -f docker-compose.security.yml up -d sonarqube-db sonarqube
  bash scripts/security/wait_sonarqube.sh || { echo "  ✗ SonarQube did not start"; FAIL=1; }
  podman run --rm --network host \
    -v "$PWD":/usr/src:z \
    -e SONAR_HOST_URL="${SONAR_HOST_URL:-http://localhost:9000}" \
    -e SONAR_TOKEN="${SONAR_TOKEN}" \
    docker.io/sonarsource/sonar-scanner-cli || FAIL=1
else
  echo "== sonarqube =="
  echo "  ⊘ SONAR_TOKEN unset, skipping"
fi

# Copy baselines
echo ""
echo "=== Copying baselines ==="
for f in gosec.json govulncheck.txt gitleaks.json trivy.json semgrep.json sbom.cdx.json snyk.json; do
  if [ -f "coverage/$f" ]; then
    cp "coverage/$f" "docs/security/baselines/scan-$f"
    echo "  → docs/security/baselines/scan-$f"
  fi
done

echo ""
if [ "$FAIL" -ne 0 ]; then
  echo "=== SCAN SUITE: FINDINGS DETECTED ==="
  exit 1
fi
echo "=== SCAN SUITE: ALL CLEAN ==="
