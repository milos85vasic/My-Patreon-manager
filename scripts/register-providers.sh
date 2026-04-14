#!/usr/bin/env bash
# scripts/register-providers.sh — auto-register LLM providers with LLMsVerifier
#
# Dynamically scans ALL environment variables matching *_API_KEY pattern,
# derives provider names from the variable names, and registers each with
# the LLMsVerifier HTTP API. Matches the HelixAgent env_scanner.go pattern
# of fully dynamic environment-variable-based auto-discovery.
#
# NO hardcoded provider list — any *_API_KEY variable with a non-empty
# value is registered automatically.
#
# Usage: bash scripts/register-providers.sh [endpoint]
# Default endpoint: http://localhost:9099
set -euo pipefail

ENDPOINT="${1:-http://localhost:9099}"
REGISTERED=0
SKIPPED=0

# Known base URLs for providers (optional optimization — unknown providers
# get a placeholder URL and the LLMsVerifier handles the rest).
# This map is loaded dynamically from the LLMsVerifier api_keys package
# conventions. New providers are still registered even without a known URL.
declare -A KNOWN_URLS=(
  [OPENROUTER]="https://openrouter.ai/api/v1"
  [GROQ]="https://api.groq.com/openai/v1"
  [DEEPSEEK]="https://api.deepseek.com/v1"
  [GEMINI]="https://generativelanguage.googleapis.com/v1beta"
  [CEREBRAS]="https://api.cerebras.ai/v1"
  [HUGGINGFACE]="https://api-inference.huggingface.co"
  [NVIDIA]="https://integrate.api.nvidia.com/v1"
  [MISTRAL]="https://api.mistral.ai/v1"
  [COHERE]="https://api.cohere.ai/v2"
  [KIMI]="https://api.moonshot.cn/v1"
  [SILICONFLOW]="https://api.siliconflow.cn/v1"
  [FIREWORKS]="https://api.fireworks.ai/inference/v1"
  [REPLICATE]="https://api.replicate.com/v1"
  [SAMBANOVA]="https://api.sambanova.ai/v1"
  [HYPERBOLIC]="https://api.hyperbolic.xyz/v1"
  [NOVITA]="https://api.novita.ai/v3"
  [UPSTAGE]="https://api.upstage.ai/v1"
  [CLOUDFLARE]="https://api.cloudflare.com/client/v4"
  [CHUTES]="https://api.chutes.ai/v1"
  [GITHUB_MODELS]="https://models.inference.ai.azure.com"
  [VENICE]="https://api.venice.ai/api/v1"
  [ZAI]="https://open.bigmodel.cn/api/paas/v4"
  [ZHIPU]="https://open.bigmodel.cn/api/paas/v4"
  [CODESTRAL]="https://codestral.mistral.ai/v1"
  [VERCEL]="https://api.vercel.ai/v1"
  [INFERENCE]="https://api.inference.net/v1"
  [NLP]="https://api.nlpcloud.io/v1"
  [PUBLICAI]="https://api.publicai.io/v1"
  [SARVAM]="https://api.sarvam.ai/v1"
  [BASETEN]="https://api.baseten.co/v1"
  [MODAL]="https://api.modal.com/v1"
)

# Derive a human-readable provider name from an env var name.
# E.g.: OPENROUTER_API_KEY → OpenRouter, HUGGINGFACE_API_KEY → HuggingFace
derive_name() {
  local var="$1"
  # Strip _API_KEY suffix
  local base="${var%_API_KEY}"
  # Also strip _API suffix (for vars like CODESTRAL_API)
  base="${base%_API}"
  # Convert UPPER_CASE to Title Case
  echo "$base" | awk '{
    n = split(tolower($0), parts, "_")
    result = ""
    for (i = 1; i <= n; i++) {
      result = result toupper(substr(parts[i], 1, 1)) substr(parts[i], 2)
      if (i < n) result = result " "
    }
    print result
  }'
}

# Derive the base URL lookup key from an env var name.
# E.g.: OPENROUTER_API_KEY → OPENROUTER, GITHUB_MODELS_API_KEY → GITHUB_MODELS
derive_url_key() {
  local var="$1"
  local base="${var%_API_KEY}"
  base="${base%_API}"
  echo "$base"
}

echo "=== Auto-registering LLM Providers ==="
echo "Endpoint: ${ENDPOINT}"
echo ""

# Dynamically scan ALL environment variables for *_API_KEY pattern.
# This matches the HelixAgent api_keys/env_scanner.go approach.
# Use process substitution to avoid subshell counter issue
while IFS='=' read -r var_name value; do
  # Skip empty values
  [ -z "$value" ] && continue

  # Skip our own project keys (not LLM providers)
  case "$var_name" in
    LLMSVERIFIER_API_KEY|PATREON_*|WEBHOOK_*|ADMIN_*|SNYK_*|SONAR_*|SEMGREP_*|TAVILY_*)
      continue
      ;;
  esac

  # Skip variable references (lines like HUGGINGFACE_API_KEY=$ApiKey_HuggingFace
  # that weren't expanded — the shell should expand them, but guard anyway)
  [[ "$value" == \$* ]] && continue

  name="$(derive_name "$var_name")"
  url_key="$(derive_url_key "$var_name")"
  base_url="${KNOWN_URLS[$url_key]:-https://api.${url_key,,}.com/v1}"

  # Register with LLMsVerifier API (idempotent — treats duplicates as success)
  http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${ENDPOINT}/api/providers" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"${name}\",\"endpoint\":\"${base_url}\",\"api_key\":\"${value}\"}" 2>/dev/null) || http_code="000"

  case "$http_code" in
    200|201)
      REGISTERED=$((REGISTERED + 1))
      echo "  OK: ${name} (${var_name})"
      ;;
    409|422)
      # Already exists or duplicate — count as success
      REGISTERED=$((REGISTERED + 1))
      echo "  OK: ${name} (${var_name}) [already registered]"
      ;;
    *)
      echo "  FAIL: ${name} (${var_name}) [HTTP ${http_code}]"
      ;;
  esac
done < <(env | grep -E '^[A-Z_]+_API_KEY=' | sort)

echo ""
echo "Registered: ${REGISTERED} providers"
echo "=== Provider Registration Complete ==="
