#!/usr/bin/env bash
set -euo pipefail
deadline=$((SECONDS + 300))
echo "Waiting for SonarQube to become healthy..."
until curl -sf http://localhost:9000/api/system/status 2>/dev/null | grep -q '"status":"UP"'; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "SonarQube did not come up within 5 minutes" >&2
    exit 1
  fi
  sleep 5
done
echo "SonarQube is UP"
