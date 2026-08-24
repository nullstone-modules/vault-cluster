#!/usr/bin/env bash
# Health of the local Vault. Prints a reason on failure.
set -euo pipefail

# shellcheck source=lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

require_cmd curl jq
load_env

FAILED=0

check() {
  local label="$1" status="$2" detail="${3:-}"
  case "${status}" in
    ok)   printf '  [ ok ]   %-28s %s\n' "${label}" "${detail}" ;;
    warn) printf '  [warn]   %-28s %s\n' "${label}" "${detail}" ;;
    *)    printf '  [FAIL]   %-28s %s\n' "${label}" "${detail}"; FAILED=1 ;;
  esac
}

printf '\nLocal Vault environment: %s\n\n' "${VAULT_ADDR}"

# --- Containers ---------------------------------------------------------------
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  running="$(compose ps --status running --format '{{.Service}}' 2>/dev/null | tr '\n' ' ' | sed 's/ $//')"
  if [ -n "${running}" ]; then
    check "containers" ok "${running}"
  else
    check "containers" fail "none running - run ./setup.sh"
  fi
else
  check "docker" warn "unavailable - checking Vault over HTTP only"
fi

# --- Vault --------------------------------------------------------------------
code="$(vault_health_code)"
if [ "${code}" = "000" ]; then
  check "vault reachable" fail "no response - run ./setup.sh"
  printf '\n'
  exit 1
fi
check "vault reachable" ok "health ${code}"

if vault_is_initialized; then
  check "initialized" ok ""
else
  check "initialized" fail "run ./setup.sh"
fi

if vault_is_sealed; then
  check "unsealed" fail "sealed - run ./setup.sh (one-shot bootstrap; not a long-running unseal sidecar)"
else
  check "unsealed" ok ""
fi

# --- Platform configuration ----------------------------------------------------
# Uses the operator token when present. Absence is a real finding, not a
# reason to skip the check silently.
#
# Probe specific tune/audit paths rather than listing sys/mounts: a 403 or
# invalid token used to look like "kv not found", and `{errors:[...]}` used
# to look like an enabled audit device because it is a non-empty JSON object.
KV_MOUNT="${KV_MOUNT:-kv}"
DATABASE_MOUNT="${DATABASE_MOUNT:-database}"

vault_probe() {
  # Prints HTTP status on stdout; body in the file named by $1.
  local body_file="$1" path="$2"
  curl -s -o "${body_file}" -w '%{http_code}' --max-time 5 \
    --header "X-Vault-Token: ${tok}" \
    "${VAULT_ADDR}/v1/${path}" 2>/dev/null || printf '000'
}

if [ -f "${BOOTSTRAP_DIR}/operator.token" ]; then
  tok="$(cat "${BOOTSTRAP_DIR}/operator.token")"
  probe_body="$(mktemp "${TMPDIR:-/tmp}/vault-health.XXXXXX")"
  chmod 600 "${probe_body}"
  trap 'rm -f "${probe_body}"' EXIT

  lookup_code="$(vault_probe "${probe_body}" "auth/token/lookup-self")"
  if [ "${lookup_code}" != "200" ]; then
    check "operator token" fail "invalid or expired (HTTP ${lookup_code}) - run ./setup.sh"
  else
    check "operator token" ok ""

    kv_code="$(vault_probe "${probe_body}" "sys/mounts/${KV_MOUNT}/tune")"
    if [ "${kv_code}" = "200" ] && jq -e '.data.options.version == "2"' "${probe_body}" >/dev/null 2>&1; then
      check "kv v2 mounted" ok "${KV_MOUNT}/"
    elif [ "${kv_code}" = "404" ]; then
      check "kv v2 mounted" fail "not found - run ./setup.sh"
    else
      check "kv v2 mounted" fail "HTTP ${kv_code} on sys/mounts/${KV_MOUNT}/tune - not a missing mount"
    fi

    audit_code="$(vault_probe "${probe_body}" "sys/audit")"
    if [ "${audit_code}" = "200" ] && jq -e '((.data // .) | to_entries | map(select(.key != "errors")) | length) > 0' "${probe_body}" >/dev/null 2>&1; then
      check "audit device" ok "enabled"
    elif [ "${audit_code}" = "403" ]; then
      check "audit device" fail "operator cannot read sys/audit"
    else
      # Vault refuses every request when no audit device can write, so this is a
      # precursor to a total outage rather than a cosmetic gap.
      check "audit device" fail "none enabled (HTTP ${audit_code})"
    fi

    db_code="$(vault_probe "${probe_body}" "sys/mounts/${DATABASE_MOUNT}/tune")"
    if [ "${db_code}" = "200" ]; then
      check "database engine" ok "${DATABASE_MOUNT}/"
    elif credentials_enabled; then
      check "database engine" fail "enabled in .env but not mounted (HTTP ${db_code})"
    else
      check "database engine" ok "disabled (isolation only)"
    fi
  fi
else
  check "operator token" warn "not found - run ./setup.sh for full checks"
fi

printf '\n'
[ "${FAILED}" -eq 0 ] || { printf 'Environment is NOT healthy.\n\n'; exit 1; }
printf 'Environment is healthy.\n\n'
