#!/usr/bin/env bash
# scripts/wait-llmsverifier.sh — polls LLMsVerifier until healthy
# Usage: bash scripts/wait-llmsverifier.sh [endpoint] [timeout_seconds]
# Defaults: endpoint=http://localhost:9090, timeout=120
set -euo pipefail

ENDPOINT="${1:-http://localhost:9099}"
TIMEOUT="${2:-120}"
deadline=$((SECONDS + TIMEOUT))

echo "Waiting for LLMsVerifier at ${ENDPOINT} (timeout: ${TIMEOUT}s)..."
until curl -sf "${ENDPOINT}/api/health" >/dev/null 2>&1; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "ERROR: LLMsVerifier did not come up within ${TIMEOUT} seconds" >&2
    exit 1
  fi
  sleep 2
done
echo "LLMsVerifier is UP at ${ENDPOINT}"
