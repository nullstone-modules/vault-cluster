#!/bin/sh
# Init once, unseal on every reseal. Not production KMS auto-unseal.
# shellcheck shell=sh

set -eu

INIT_FILE="${INIT_FILE:-/bootstrap/vault-init.json}"
SHARES="${VAULT_INIT_KEY_SHARES:-5}"
THRESHOLD="${VAULT_INIT_KEY_THRESHOLD:-3}"
VAULT_ADDR="${VAULT_ADDR:-http://vault:8200}"
export VAULT_ADDR
export VAULT_CLI_NO_COLOR=true

log() { printf '[unseal] %s\n' "$*" >&2; }

mkdir -p /bootstrap

# Bind-mount owner on the host. Writing as root (user 0:0) would leave
# vault-init.json mode 600 owned by root, which GitHub Actions cannot jq.
bootstrap_uid=$(stat -c '%u' /bootstrap 2>/dev/null || printf '0')
bootstrap_gid=$(stat -c '%g' /bootstrap 2>/dev/null || printf '0')

own_bootstrap() {
  chown "${bootstrap_uid}:${bootstrap_gid}" /bootstrap "$@" 2>/dev/null || true
}

chmod 700 /bootstrap 2>/dev/null || true
own_bootstrap

wait_for_vault() {
  n=0
  while [ "${n}" -lt 60 ]; do
    ec=0
    vault status >/dev/null 2>&1 || ec=$?
    if [ "${ec}" -eq 0 ] || [ "${ec}" -eq 2 ]; then
      return 0
    fi
    sleep 1
    n=$((n + 1))
  done
  return 1
}

is_initialized() {
  vault status 2>/dev/null | grep 'Initialized' | grep -q 'true'
}

is_sealed() {
  vault status 2>/dev/null | grep 'Sealed' | grep -q 'true'
}

# Print Shamir shares, one per line. Supports HTTP-init (keys_base64) and
# CLI-init (unseal_keys_b64). Output must never be logged.
print_unseal_keys() {
  _buf=$(tr -d '\n' < "$1")
  case "${_buf}" in
    *'"unseal_keys_b64"'*) _rest=${_buf#*\"unseal_keys_b64\"} ;;
    *'"keys_base64"'*)     _rest=${_buf#*\"keys_base64\"} ;;
    *) return 1 ;;
  esac
  _rest=${_rest#*[}
  _rest=${_rest%%]*}
  _oldifs=${IFS}
  IFS=,
  # shellcheck disable=SC2086
  set -- ${_rest}
  IFS=${_oldifs}
  for _k in "$@"; do
    _k=$(printf '%s' "${_k}" | tr -d ' "')
    [ -n "${_k}" ] && printf '%s\n' "${_k}"
  done
}

do_init() {
  tmp="${INIT_FILE}.tmp.$$"
  umask 077
  if vault operator init \
       -key-shares="${SHARES}" \
       -key-threshold="${THRESHOLD}" \
       -format=json > "${tmp}"; then
    mv "${tmp}" "${INIT_FILE}"
    chmod 600 "${INIT_FILE}"
    own_bootstrap "${INIT_FILE}"
    log "initialized persistent Vault; unseal material written (not logged)"
    log "BACK UP ${INIT_FILE} on the host. Without it this volume cannot be unsealed."
  else
    rm -f "${tmp}"
    log "init did not run (already initialized, or Vault not ready)"
  fi
}

do_unseal() {
  [ -f "${INIT_FILE}" ] || return 1
  n=0
  print_unseal_keys "${INIT_FILE}" | while IFS= read -r key; do
    [ -n "${key}" ] || continue
    n=$((n + 1))
    if [ "${n}" -le "${THRESHOLD}" ]; then
      # Exit 2 while still sealed is expected until threshold shares are in.
      vault operator unseal "${key}" >/dev/null || true
    fi
  done
}

log "local Shamir unseal sidecar (not production KMS auto-unseal)"
log "VAULT_ADDR=${VAULT_ADDR}"

while true; do
  if ! wait_for_vault; then
    log "Vault not responding; retrying"
    sleep 5
    continue
  fi

  if ! is_initialized; then
    log "Vault is uninitialized; running operator init"
    do_init
    sleep 1
    continue
  fi

  if is_sealed; then
    if [ ! -f "${INIT_FILE}" ]; then
      log "sealed, but unseal material is missing from ${INIT_FILE}"
      log "restore the host .bootstrap/vault-init.json, or reset the volume"
      sleep 5
      continue
    fi
    log "Vault is sealed; submitting local Shamir shares"
    if do_unseal && ! is_sealed; then
      log "unsealed"
    else
      # Raft can reseal briefly after init while a leader is elected.
      sleep 2
      if is_sealed; then
        do_unseal || true
      fi
      if is_sealed; then
        log "still sealed; will retry"
      else
        log "unsealed"
      fi
    fi
  fi

  sleep 5
done
