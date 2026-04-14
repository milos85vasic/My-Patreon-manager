#!/usr/bin/env bash
# scripts/llmsverifier.sh — start LLMsVerifier, wait for health, refresh .env
#
# Usage:
#   bash scripts/llmsverifier.sh          # start and update .env
#   bash scripts/llmsverifier.sh stop      # stop the container
#   bash scripts/llmsverifier.sh status    # check if running
#
# The script:
#   1. Generates a fresh API key on every start (rotated each boot)
#   2. Starts the LLMsVerifier container via docker-compose
#   3. Waits until the /v1/models endpoint responds
#   4. Updates LLMSVERIFIER_ENDPOINT and LLMSVERIFIER_API_KEY in .env
#
# Environment overrides:
#   LLMSVERIFIER_PORT        — host port (default: 9090)
#   LLMSVERIFIER_TIMEOUT     — health check timeout in seconds (default: 120)
#   COMPOSE_CMD              — docker compose command (default: auto-detect)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${ROOT}/.env"
PORT="${LLMSVERIFIER_PORT:-9090}"
TIMEOUT="${LLMSVERIFIER_TIMEOUT:-120}"
ENDPOINT="http://localhost:${PORT}"
COMPOSE_FILE="${ROOT}/docker-compose.yml"

# ---------------------------------------------------------------------------
# Auto-detect compose command (docker compose v2, docker-compose v1, podman)
# ---------------------------------------------------------------------------
detect_compose() {
  if [ -n "${COMPOSE_CMD:-}" ]; then
    echo "${COMPOSE_CMD}"
    return
  fi
  if docker compose version >/dev/null 2>&1; then
    echo "docker compose"
  elif docker-compose version >/dev/null 2>&1; then
    echo "docker-compose"
  elif podman-compose version >/dev/null 2>&1; then
    echo "podman-compose"
  else
    echo "ERROR: no docker compose, docker-compose, or podman-compose found" >&2
    exit 1
  fi
}

COMPOSE="$(detect_compose)"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
generate_api_key() {
  # 32-byte random hex string — fresh on every boot for security
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  elif [ -r /dev/urandom ]; then
    head -c 32 /dev/urandom | xxd -p -c 64
  else
    python3 -c "import secrets; print(secrets.token_hex(32))"
  fi
}

# Update or append a KEY=VALUE pair in the .env file.
# Does NOT create the file — it must already exist.
set_env_var() {
  local key="$1" value="$2" file="$3"
  if grep -q "^${key}=" "$file" 2>/dev/null; then
    # Replace existing line (portable sed -i)
    local tmp
    tmp="$(mktemp)"
    sed "s|^${key}=.*|${key}=${value}|" "$file" > "$tmp" && mv "$tmp" "$file"
  else
    # Append with a section header if this is the first LLMsVerifier entry
    if ! grep -q "LLMSVERIFIER" "$file" 2>/dev/null; then
      printf '\n# LLMsVerifier (auto-managed by scripts/llmsverifier.sh)\n' >> "$file"
    fi
    echo "${key}=${value}" >> "$file"
  fi
}

# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------
cmd_start() {
  echo "=== LLMsVerifier Bootstrap ==="

  # 1. Generate a fresh API key
  local api_key
  api_key="$(generate_api_key)"
  echo "  Generated fresh API key (rotated on every start)"

  # 2. Export for docker-compose interpolation
  export LLMSVERIFIER_API_KEY="${api_key}"
  export LLMSVERIFIER_PORT="${PORT}"

  # 3. Start the container
  echo "  Starting LLMsVerifier container on port ${PORT}..."
  ${COMPOSE} -f "${COMPOSE_FILE}" up -d llmsverifier

  # 4. Wait for health
  bash "${ROOT}/scripts/wait-llmsverifier.sh" "${ENDPOINT}" "${TIMEOUT}"

  # 5. Update .env with fresh values
  if [ ! -f "$ENV_FILE" ]; then
    echo "ERROR: ${ENV_FILE} not found — copy .env.example to .env first" >&2
    exit 1
  fi

  set_env_var "LLMSVERIFIER_ENDPOINT" "${ENDPOINT}" "${ENV_FILE}"
  set_env_var "LLMSVERIFIER_API_KEY"  "${api_key}"  "${ENV_FILE}"

  echo ""
  echo "  .env updated:"
  echo "    LLMSVERIFIER_ENDPOINT=${ENDPOINT}"
  echo "    LLMSVERIFIER_API_KEY=<rotated>"
  echo ""
  echo "=== LLMsVerifier is READY ==="
  echo ""
  echo "  Verify:  go run ./cmd/cli verify"
  echo "  Dry-run: go run ./cmd/cli sync --dry-run"
}

cmd_stop() {
  echo "Stopping LLMsVerifier..."
  ${COMPOSE} -f "${COMPOSE_FILE}" stop llmsverifier
  ${COMPOSE} -f "${COMPOSE_FILE}" rm -f llmsverifier
  echo "LLMsVerifier stopped"
}

cmd_status() {
  if curl -sf "${ENDPOINT}/v1/models" >/dev/null 2>&1; then
    echo "LLMsVerifier is UP at ${ENDPOINT}"
    exit 0
  else
    echo "LLMsVerifier is NOT reachable at ${ENDPOINT}"
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Dispatch
# ---------------------------------------------------------------------------
case "${1:-start}" in
  start)  cmd_start  ;;
  stop)   cmd_stop   ;;
  status) cmd_status ;;
  *)
    echo "Usage: $0 {start|stop|status}" >&2
    exit 1
    ;;
esac
