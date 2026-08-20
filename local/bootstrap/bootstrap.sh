#!/usr/bin/env bash
# ./bootstrap.sh [--keep-root]
set -euo pipefail

# shellcheck source=lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

KEEP_ROOT=false
while [ $# -gt 0 ]; do
  case "$1" in
    --keep-root) KEEP_ROOT=true ;;
    -h|--help) sed -n '2,2p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
  shift
done

require_cmd curl jq
load_env
ensure_bootstrap_dir

wait_for_vault 60
wait_for_unsealed 90

# 1. Initialize (Compose sidecar normally already did this)
if vault_is_initialized; then
  info "Vault is already initialized (skipping init)"
  [ -f "${INIT_FILE}" ] || die \
    "Vault is initialized but ${INIT_FILE} is missing. The unseal shares for
     this storage volume are gone, so it cannot be unsealed. Either restore
     that file from wherever it was kept, or discard the data with:
       ./reset.sh --yes"
else
  info "initializing Vault (${VAULT_INIT_KEY_SHARES} shares, threshold ${VAULT_INIT_KEY_THRESHOLD})"

  # umask before the write, not chmod after: chmod leaves a window in which the
  # file exists with default permissions, and unseal keys must never be
  # world-readable for even an instant.
  (
    umask 077
    curl -sf --max-time 30 \
      --request POST \
      --data "$(jq -nc \
          --argjson shares "${VAULT_INIT_KEY_SHARES}" \
          --argjson threshold "${VAULT_INIT_KEY_THRESHOLD}" \
          '{secret_shares: $shares, secret_threshold: $threshold}')" \
      "${VAULT_ADDR}/v1/sys/init" > "${INIT_FILE}"
  ) || die "vault init failed"

  # Fail loudly rather than leaving a truncated key file that looks valid.
  jq -e '.root_token and ((.keys_base64 // .unseal_keys_b64) | length > 0)' "${INIT_FILE}" >/dev/null 2>&1 \
    || { rm -f "${INIT_FILE}"; die "init response did not contain keys - refusing to continue"; }

  info "initialized. Unseal material written to ${INIT_FILE} (mode 600, not logged)"
  warn "BACK THIS FILE UP. Without it this Vault cannot be unsealed."
fi

# 2. Unseal
unseal_vault

# Skip only when isolation is up and, if requested, the database engine is too.
# Isolation-then-credentials must not exit here or the DB engine never mounts.
if platform_appears_configured; then
  if ! credentials_enabled || database_engine_mounted; then
    info "platform already configured - skipping bootstrap configuration"
    cat >&2 <<EOF

--------------------------------------------------------------------
Local environment is ready (already initialized).

  VAULT_ADDR   ${VAULT_ADDR}
  capability   isolation$(credentials_enabled && printf ' + dynamic database credentials')
  tokens       ${BOOTSTRAP_DIR}/provisioning.token
               ${BOOTSTRAP_DIR}/operator.token
--------------------------------------------------------------------
EOF
    exit 0
  fi
  info "isolation is configured; enabling dynamic credentials"
fi

# 3. Configure the platform.
# From here this script is a caller, not an implementer. Everything below is
# generic Vault configuration that knows nothing about Docker; the target only
# supplies runtime-specific values (where the audit log goes, how to reach
# PostgreSQL) as environment variables.

# Rebuild a root token from unseal shares when the stored one was revoked.
# Local-only: production root recovery is the break-glass runbook. Needed here
# because bootstrap is documented as re-runnable after root is retired.
regenerate_root_from_shares() {
  info "stored root token is unusable - generating a temporary root from unseal shares"
  curl -s --max-time 10 --request DELETE \
    "${VAULT_ADDR}/v1/sys/generate-root/attempt" >/dev/null 2>&1 || true

  local otp nonce attempt encoded="" http body
  body="$(mktemp "${TMPDIR:-/tmp}/vault-genroot.XXXXXX")"
  chmod 600 "${body}"

  # Let Vault mint the OTP. A host-generated OTP is rejected unless it matches
  # Vault's exact encoding, and curl -f hid that 400.
  http="$(curl -s -o "${body}" -w '%{http_code}' --max-time 10 \
    --request PUT --header 'Content-Type: application/json' --data '{}' \
    "${VAULT_ADDR}/v1/sys/generate-root/attempt")"
  attempt="$(cat "${body}")"
  [ "${http}" = "200" ] || { rm -f "${body}"; die "generate-root attempt failed (HTTP ${http}): $(printf '%s' "${attempt}" | jq -rc '.errors // .')"; }
  nonce="$(printf '%s' "${attempt}" | jq -r '.nonce')"
  otp="$(printf '%s' "${attempt}" | jq -r '.otp')"
  [ -n "${nonce}" ] && [ "${nonce}" != "null" ] || die "generate-root did not return a nonce"
  [ -n "${otp}" ] && [ "${otp}" != "null" ] || die "generate-root did not return an otp"

  local key
  while IFS= read -r key; do
    http="$(curl -s -o "${body}" -w '%{http_code}' --max-time 10 \
      --request PUT --header 'Content-Type: application/json' \
      --data "$(jq -nc --arg k "${key}" --arg n "${nonce}" '{key: $k, nonce: $n}')" \
      "${VAULT_ADDR}/v1/sys/generate-root/update")"
    attempt="$(cat "${body}")"
    [ "${http}" = "200" ] || die "generate-root update failed (HTTP ${http}): $(printf '%s' "${attempt}" | jq -rc '.errors // .')"
    if [ "$(printf '%s' "${attempt}" | jq -r '.complete')" = "true" ]; then
      encoded="$(printf '%s' "${attempt}" | jq -r '.encoded_root_token // .encoded_token')"
      break
    fi
  done < <(jq -r "(.keys_base64 // .unseal_keys_b64)[:${VAULT_INIT_KEY_THRESHOLD}][]" "${INIT_FILE}")
  rm -f "${body}"

  [ -n "${encoded}" ] && [ "${encoded}" != "null" ] \
    || die "generate-root did not produce an encoded token"

  # Decode with the same Vault binary that minted the OTP so padding/alphabet
  # cannot drift across Python base64 variants.
  compose exec -T vault vault operator generate-root \
    -decode="${encoded}" -otp="${otp}" | tr -d '\r\n'
}

VAULT_TOKEN="$(jq -r '.root_token' "${INIT_FILE}")"
if ! token_is_usable "${VAULT_TOKEN}"; then
  VAULT_TOKEN="$(regenerate_root_from_shares)"
  token_is_usable "${VAULT_TOKEN}" || die "generated root token is not usable"
fi
export VAULT_TOKEN

# /vault/logs is a separate volume from /vault/file (Raft storage), so audit
# growth cannot threaten the storage backend.
export AUDIT_LOG_PATH="/vault/logs/audit.log"

if credentials_enabled; then
  # Vault reaches PostgreSQL over the container network by service name; the
  # host does not appear anywhere in this address.
  export DATABASE_CONNECTION_URL="postgresql://{{username}}:{{password}}@postgres:5432/${POSTGRES_DB:-appdb}?sslmode=disable"
  export DATABASE_USERNAME="vault_admin"
  export DATABASE_PASSWORD="${VAULT_DB_ADMIN_PASSWORD:?VAULT_DB_ADMIN_PASSWORD must be set in .env}"
fi

info "applying platform configuration"
"${REPO_ROOT}/scripts/configure-platform.sh"

# 4. Issue working identities.
# Periodic tokens: renewable indefinitely while in use, dead shortly after they
# stop being renewed. A non-expiring token would be a permanent credential
# sitting on a laptop.
issue_token() {
  local policy="$1" outfile="${BOOTSTRAP_DIR}/$2" token
  # Orphan: these tokens must outlive the root token. Vault revokes every
  # child when the parent is revoked, and step 5 retires root on purpose.
  # Without orphan:true, bootstrap writes operator/provisioning tokens that
  # are already dead by the time health.sh runs.
  token="$(curl -sf --max-time 10 \
    --header "X-Vault-Token: ${VAULT_TOKEN}" \
    --request POST \
    --data "$(jq -nc --arg p "${policy}" \
        '{policies: [$p], period: "24h", renewable: true, display_name: $p, orphan: true, no_parent: true}')" \
    "${VAULT_ADDR}/v1/auth/token/create" | jq -r '.auth.client_token')"

  [ -n "${token}" ] && [ "${token}" != "null" ] || die "failed to issue ${policy} token"

  ( umask 077; printf '%s' "${token}" > "${outfile}" )
  info "issued ${policy} token -> ${outfile} (mode 600)"
}

issue_token "provisioning" "provisioning.token"
issue_token "operator"     "operator.token"

# 5. Retire the root token.
# The initial root token is bootstrap material, not an operating credential. It
# is revoked here so it cannot drift into daily use or a shell history. It can
# be regenerated from a quorum of unseal shares, which is deliberately an
# auditable and inconvenient act.
if [ "${KEEP_ROOT}" = "true" ]; then
  warn "keeping the root token active (--keep-root). Do not use this outside debugging."
else
  info "revoking the initial root token"
  curl -sf --max-time 10 \
    --header "X-Vault-Token: ${VAULT_TOKEN}" \
    --request POST \
    "${VAULT_ADDR}/v1/auth/token/revoke-self" >/dev/null \
    || warn "root token revocation failed - revoke it manually"

  # The init file still holds the (now revoked) root token string. Blank it so a
  # stale value cannot be pasted into a shell later and fail confusingly. The
  # unseal shares are retained: they are the recovery path.
  ( umask 077
    jq '.root_token = "revoked-at-bootstrap"' "${INIT_FILE}" > "${INIT_FILE}.tmp" \
      && mv "${INIT_FILE}.tmp" "${INIT_FILE}" )
fi

cat >&2 <<EOF

--------------------------------------------------------------------
Bootstrap complete.

  VAULT_ADDR   ${VAULT_ADDR}
  capability   isolation$(credentials_enabled && printf ' + dynamic database credentials')
  tokens       ${BOOTSTRAP_DIR}/provisioning.token
               ${BOOTSTRAP_DIR}/operator.token
  unseal keys  ${INIT_FILE}   <- back this up

Next:
  export VAULT_ADDR=${VAULT_ADDR}
  export VAULT_TOKEN=\$(cat ${BOOTSTRAP_DIR}/provisioning.token)
  ${REPO_ROOT}/scripts/tenants/create-tenant.sh tenant-a
--------------------------------------------------------------------
EOF
