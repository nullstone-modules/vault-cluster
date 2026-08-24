#!/usr/bin/env bash
# Shared helpers for the local Docker target.
#
# Sourced, not executed. Callers set their own `set -euo pipefail`.
#
# Everything Docker-specific in this repository lives under local/. A helper
# needed inside config/, scripts/, or tests/ is a signal the logic belongs
# there instead, or should be expressed against VAULT_ADDR rather than against
# a container.

# shellcheck shell=bash

ADAPTER_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd -- "${ADAPTER_DIR}/.." && pwd)"
COMPOSE_FILE="${ADAPTER_DIR}/docker-compose.yml"
ENV_FILE="${ADAPTER_DIR}/.env"
ENV_EXAMPLE="${ADAPTER_DIR}/.env.example"
BOOTSTRAP_DIR="${ADAPTER_DIR}/.bootstrap"
INIT_FILE="${BOOTSTRAP_DIR}/vault-init.json"

export ADAPTER_DIR REPO_ROOT COMPOSE_FILE ENV_FILE BOOTSTRAP_DIR INIT_FILE

# --- Output ------------------------------------------------------------------
# Diagnostics go to stderr so a script's stdout stays machine-readable.

_ts() { date -u '+%H:%M:%S'; }

info() { printf '[%s] %s\n'        "$(_ts)" "$*" >&2; }
warn() { printf '[%s] WARN: %s\n'  "$(_ts)" "$*" >&2; }
die()  { printf '[%s] ERROR: %s\n' "$(_ts)" "$*" >&2; exit 1; }

# --- Preconditions -----------------------------------------------------------

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

# --- Configuration -----------------------------------------------------------

load_env() {
  if [ ! -f "${ENV_FILE}" ]; then
    info "no .env found, creating one from .env.example"
    cp "${ENV_EXAMPLE}" "${ENV_FILE}"
  fi

  # ./setup.sh --with-credentials sets this before load_env; .env defaults to false.
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

# True when dynamic database credentials are enabled.
credentials_enabled() {
  [ "${ENABLE_DYNAMIC_CREDENTIALS:-false}" = "true" ]
}

# Compose profiles select which services exist at all. With credentials
# disabled, Vault runs alone; PostgreSQL is not started, not just unused.
compose() {
  # Bash 3.2 (macOS /bin/bash) treats an empty "${array[@]}" as unbound under
  # `set -u`, which is why up.sh died on `compose pull` with credentials off.
  if credentials_enabled; then
    docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" --profile credentials "$@"
  else
    docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" "$@"
  fi
}

# --- Vault reachability -------------------------------------------------------
# Uses the HTTP API rather than the Vault CLI, so the CLI is not a prerequisite
# for any target or platform script.

# Health status codes carry meaning: 200 unsealed/active, 501 uninitialized,
# 503 sealed. All three mean "Vault is answering", which is what waiting is for.
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

# Host scripts wait after the vault-utils one-shot so they do not race init.
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
  die "Vault was not unsealed within ${timeout}s. Re-run ./setup.sh (one-shot bootstrap)."
}

vault_is_initialized() {
  [ "$(curl -s --max-time 3 "${VAULT_ADDR}/v1/sys/init" | jq -r '.initialized // false')" = "true" ]
}

vault_is_sealed() {
  # Do not use `// true`: jq's // also replaces JSON false, so an unsealed
  # Vault would still look sealed.
  [ "$(curl -s --max-time 3 "${VAULT_ADDR}/v1/sys/seal-status" | jq -r '.sealed')" = "true" ]
}

# --- Bootstrap material -------------------------------------------------------
# Unseal shares and the initial root token. Restrictive permissions are applied
# to the directory as well as the files, because a readable directory leaks
# filenames and an over-permissive one lets a later write land unprotected.

ensure_bootstrap_dir() {
  mkdir -p "${BOOTSTRAP_DIR}"
  chmod 700 "${BOOTSTRAP_DIR}"
}

read_bootstrap_token() {
  local name="$1"
  local file="${BOOTSTRAP_DIR}/${name}"
  [ -f "${file}" ] || die "missing ${file} - run ./setup.sh first"
  cat "${file}"
}

# True when a token string is accepted by lookup-self. Never prints the token.
token_is_usable() {
  local t="${1:-}"
  [ -n "${t}" ] && [ "${t}" != "null" ] && [ "${t}" != "revoked-at-bootstrap" ] || return 1
  [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
    --header "X-Vault-Token: ${t}" \
    "${VAULT_ADDR}/v1/auth/token/lookup-self" 2>/dev/null || echo 000)" = "200" ]
}

operator_token_usable() {
  [ -f "${BOOTSTRAP_DIR}/operator.token" ] || return 1
  token_is_usable "$(cat "${BOOTSTRAP_DIR}/operator.token")"
}

provisioning_token_usable() {
  [ -f "${BOOTSTRAP_DIR}/provisioning.token" ] || return 1
  token_is_usable "$(cat "${BOOTSTRAP_DIR}/provisioning.token")"
}

# Operator can read mount tune; success means the platform has been configured.
platform_appears_configured() {
  operator_token_usable || return 1
  local tok code
  tok="$(cat "${BOOTSTRAP_DIR}/operator.token")"
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
    --header "X-Vault-Token: ${tok}" \
    "${VAULT_ADDR}/v1/sys/mounts/${KV_MOUNT:-kv}/tune" 2>/dev/null || echo 000)"
  [ "${code}" = "200" ]
}

database_engine_mounted() {
  operator_token_usable || return 1
  local tok code
  tok="$(cat "${BOOTSTRAP_DIR}/operator.token")"
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
    --header "X-Vault-Token: ${tok}" \
    "${VAULT_ADDR}/v1/sys/mounts/${DATABASE_MOUNT:-database}/tune" 2>/dev/null || echo 000)"
  [ "${code}" = "200" ]
}

# Submit Shamir shares from INIT_FILE. Local development automation only —
# this is not production KMS auto-unseal. Shares are never printed.
unseal_vault() {
  [ -f "${INIT_FILE}" ] || die \
    "cannot unseal: ${INIT_FILE} is missing. Restore it, or discard the
     volume with ./reset.sh --yes"

  if ! vault_is_sealed; then
    info "Vault is already unsealed"
    return 0
  fi

  info "unsealing with local Shamir shares (not production KMS auto-unseal)"
  while IFS= read -r key; do
    [ -n "${key}" ] || continue
    curl -sf --max-time 10 --request POST \
      --data "$(jq -nc --arg k "${key}" '{key: $k}')" \
      "${VAULT_ADDR}/v1/sys/unseal" >/dev/null || die "unseal step failed"
    vault_is_sealed || break
  done < <(jq -r "(.keys_base64 // .unseal_keys_b64)[:${VAULT_INIT_KEY_THRESHOLD}][]" "${INIT_FILE}")

  # Raft reseals briefly after init while it elects a leader.
  local waited=0
  while vault_is_sealed && [ "${waited}" -lt 20 ]; do
    sleep 1
    waited=$((waited + 1))
  done

  vault_is_sealed && die "Vault is still sealed after applying local unseal shares"
  info "unsealed"
}
