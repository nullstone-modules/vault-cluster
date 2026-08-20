#!/usr/bin/env bash
# HTTP helpers. Use status codes (403 vs 404); the Vault CLI collapses both.
# shellcheck shell=bash

VAULT_HTTP_TIMEOUT="${VAULT_HTTP_TIMEOUT:-15}"

_VAULT_BODY_FILE=""

# Call directly, never via $() — a subshell trap would delete the parent's body file.
_vault_ensure_body_file() {
  if [ -z "${_VAULT_BODY_FILE:-}" ] || [ ! -f "${_VAULT_BODY_FILE}" ]; then
    _VAULT_BODY_FILE="$(mktemp "${TMPDIR:-/tmp}/vault-resp.XXXXXX")"
    chmod 600 "${_VAULT_BODY_FILE}"
    # Skip cleanup in subshells so a pipeline cannot delete the parent's file.
    if [ "${BASH_SUBSHELL:-0}" -eq 0 ]; then
      trap '[ -n "${_VAULT_BODY_FILE:-}" ] && rm -f "${_VAULT_BODY_FILE}"' EXIT
    fi
  fi
}

vault_require_env() {
  [ -n "${VAULT_ADDR:-}"  ] || die "VAULT_ADDR is not set"
  [ -n "${VAULT_TOKEN:-}" ] || die "VAULT_TOKEN is not set"
}

vault_request() {
  local method="$1" path="$2" data="${3:-}"
  local url="${VAULT_ADDR%/}/v1/${path#/}"
  _vault_ensure_body_file
  local body_file="${_VAULT_BODY_FILE}"
  local args=(
    --silent --show-error
    --max-time "${VAULT_HTTP_TIMEOUT}"
    --output "${body_file}"
    --write-out '%{http_code}'
    --request "${method}"
    --header "X-Vault-Token: ${VAULT_TOKEN}"
  )
  [ -n "${VAULT_NAMESPACE:-}" ] && args+=(--header "X-Vault-Namespace: ${VAULT_NAMESPACE}")
  if [ -n "${data}" ]; then
    args+=(--header 'Content-Type: application/json' --data "${data}")
  fi

  VAULT_STATUS="$(curl "${args[@]}" "${url}" 2>/dev/null || printf '000')"
  export VAULT_STATUS

  case "${VAULT_STATUS}" in
    2*) return 0 ;;
    *)  return 1 ;;
  esac
}

vault_body() {
  _vault_ensure_body_file
  cat "${_VAULT_BODY_FILE}" 2>/dev/null || printf '{}'
}

vault_status_of() {
  vault_request "$@" >/dev/null 2>&1 || true
  printf '%s' "${VAULT_STATUS}"
}

vault_get()    { vault_request GET    "$1"; }
vault_post()   { vault_request POST   "$1" "${2:-}"; }
vault_put()    { vault_request PUT    "$1" "${2:-}"; }
vault_delete() { vault_request DELETE "$1"; }
vault_list()   { vault_request GET    "$1?list=true"; }

vault_must() {
  local method="$1" path="$2" data="${3:-}"
  if ! vault_request "${method}" "${path}" "${data}"; then
    die "${method} ${path} failed (HTTP ${VAULT_STATUS}): $(vault_body | jq -rc '.errors // .' 2>/dev/null || vault_body)"
  fi
}

vault_mount_exists() {
  vault_request GET "sys/mounts/$1/tune" >/dev/null 2>&1
}

vault_auth_exists() {
  vault_request GET "sys/auth" >/dev/null 2>&1 || return 1
  vault_body | jq -e --arg m "$1/" '(.data // .) | has($m)' >/dev/null 2>&1
}

vault_audit_device_exists() {
  vault_request GET "sys/audit" >/dev/null 2>&1 || return 1
  vault_body | jq -e --arg m "$1/" '(.data // .) | has($m)' >/dev/null 2>&1
}

vault_write_policy() {
  local name="$1" file="$2"
  [ -f "${file}" ] || die "policy file not found: ${file}"
  vault_must PUT "sys/policies/acl/${name}" \
    "$(jq -n --rawfile p "${file}" '{policy: $p}')"
}

vault_policy_exists() {
  vault_request GET "sys/policies/acl/$1" >/dev/null 2>&1
}
