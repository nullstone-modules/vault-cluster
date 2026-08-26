#!/usr/bin/env bash
# shellcheck shell=bash

ADAPTER_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${ADAPTER_DIR}/.." && pwd)"
COMPOSE_FILE="${ADAPTER_DIR}/compose.yml"
ENV_FILE="${ADAPTER_DIR}/.env"
ENV_EXAMPLE="${ADAPTER_DIR}/.env.example"
BOOTSTRAP_DIR="${ADAPTER_DIR}/.bootstrap"
INIT_FILE="${BOOTSTRAP_DIR}/vault-init.json"

export ADAPTER_DIR REPO_ROOT COMPOSE_FILE ENV_FILE BOOTSTRAP_DIR INIT_FILE

_ts() { date -u '+%H:%M:%S'; }
info() { printf '[%s] %s\n'        "$(_ts)" "$*" >&2; }
warn() { printf '[%s] WARN: %s\n'  "$(_ts)" "$*" >&2; }
die()  { printf '[%s] ERROR: %s\n' "$(_ts)" "$*" >&2; exit 1; }

require_cmd() {
  local missing=0 c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || { warn "required command not found: $c"; missing=1; }
  done
  [ "$missing" -eq 0 ] || die "install the missing dependencies listed above, then retry"
}

require_docker() {
  require_cmd docker
  docker compose version >/dev/null 2>&1 \
    || die "Docker Compose v2 is required (got: $(docker --version 2>/dev/null || echo 'no docker'))"
  docker info >/dev/null 2>&1 \
    || die "the Docker daemon is not running - start Docker Desktop and retry"
}

load_env() {
  if [ ! -f "${ENV_FILE}" ]; then
    info "no .env found, creating one from .env.example"
    cp "${ENV_EXAMPLE}" "${ENV_FILE}"
  fi
  local creds_from_cli="${ENABLE_DYNAMIC_CREDENTIALS:-}"
  set -a
  # shellcheck disable=SC1090
  . "${ENV_FILE}"
  set +a
  if [ "${creds_from_cli}" = "true" ]; then
    ENABLE_DYNAMIC_CREDENTIALS=true
  fi
  : "${VAULT_HOST_PORT:=8200}"
  : "${ENABLE_DYNAMIC_CREDENTIALS:=false}"
  : "${VAULT_INIT_KEY_SHARES:=5}"
  : "${VAULT_INIT_KEY_THRESHOLD:=3}"
  VAULT_ADDR="http://127.0.0.1:${VAULT_HOST_PORT}"
  export VAULT_ADDR ENABLE_DYNAMIC_CREDENTIALS
}

credentials_enabled() {
  [ "${ENABLE_DYNAMIC_CREDENTIALS:-false}" = "true" ]
}

compose() {
  if credentials_enabled; then
    docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" --profile credentials "$@"
  else
    docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" "$@"
  fi
}

vault_health_code() {
  curl -s -o /dev/null -w '%{http_code}' --max-time 3 \
    "${VAULT_ADDR}/v1/sys/health?standbyok=true" 2>/dev/null || echo "000"
}

wait_for_vault() {
  local timeout="${1:-60}" waited=0 code
  info "waiting for Vault at ${VAULT_ADDR} (timeout ${timeout}s)"
  while [ "${waited}" -lt "${timeout}" ]; do
    code="$(vault_health_code)"
    case "${code}" in
      200|429|472|473|501|503)
        info "Vault is responding (health ${code})"
        return 0
        ;;
    esac
    sleep 1
    waited=$((waited + 1))
  done
  die "Vault did not respond within ${timeout}s (last health code: ${code:-none})"
}

wait_for_unsealed() {
  local timeout="${1:-90}" waited=0
  info "waiting for Vault to be initialized and unsealed (timeout ${timeout}s)"
  while [ "${waited}" -lt "${timeout}" ]; do
    if vault_is_initialized && ! vault_is_sealed; then
      info "Vault is initialized and unsealed"
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  die "Vault was not unsealed within ${timeout}s. Re-run ./setup.sh"
}

vault_is_initialized() {
  [ "$(curl -s --max-time 3 "${VAULT_ADDR}/v1/sys/init" | jq -r '.initialized // false')" = "true" ]
}

vault_is_sealed() {
  [ "$(curl -s --max-time 3 "${VAULT_ADDR}/v1/sys/seal-status" | jq -r '.sealed')" = "true" ]
}

ensure_bootstrap_dir() {
  mkdir -p "${BOOTSTRAP_DIR}"
  chmod 700 "${BOOTSTRAP_DIR}"
}
