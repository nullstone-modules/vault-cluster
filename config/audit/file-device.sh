#!/usr/bin/env bash
# Enable file audit. log_raw stays false.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../scripts/lib/common.sh
. "${SCRIPT_DIR}/../../scripts/lib/common.sh"
# shellcheck source=../../scripts/lib/vault.sh
. "${SCRIPT_DIR}/../../scripts/lib/vault.sh"

platform_defaults
vault_require_env

: "${AUDIT_LOG_PATH:?AUDIT_LOG_PATH must be supplied by the deployment target}"
AUDIT_DEVICE="${AUDIT_DEVICE_NAME:-file}"

if [ "${ENABLE_AUDIT}" != "true" ]; then
  # Vault refuses every request when no audit device can write. Running without
  # one is therefore not "less logging", it is an unmonitored secrets store.
  warn "ENABLE_AUDIT is false - skipping. Do not do this outside a throwaway environment."
  exit 0
fi

if vault_audit_device_exists "${AUDIT_DEVICE}"; then
  info "audit device '${AUDIT_DEVICE}' already enabled"
  exit 0
fi

info "enabling file audit device at ${AUDIT_LOG_PATH}"

# log_raw=false: HMAC values, never raw secrets.
vault_must PUT "sys/audit/${AUDIT_DEVICE}" "$(jq -nc \
  --arg path "${AUDIT_LOG_PATH}" \
  '{
     type: "file",
     description: "Platform audit device",
     options: {
       file_path: $path,
       log_raw: "false",
       hmac_accessor: "true",
       mode: "0600",
       format: "json"
     }
   }')"

info "audit device enabled"
