#!/usr/bin/env bash
# Enable AppRole.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib/common.sh
. "${SCRIPT_DIR}/../../lib/common.sh"
# shellcheck source=../../lib/vault.sh
. "${SCRIPT_DIR}/../../lib/vault.sh"

platform_defaults
vault_require_env

if vault_auth_exists "${AUTH_MOUNT}"; then
  info "auth method '${AUTH_MOUNT}/' already enabled"
else
  info "enabling AppRole auth at '${AUTH_MOUNT}/'"
  vault_must POST "sys/auth/${AUTH_MOUNT}" "$(jq -nc \
    '{
       type: "approle",
       description: "Tenant workload identities"
     }')"
fi

# Mount-level TTL ceiling. Per-role TTLs may be shorter but cannot exceed this,
# so a mistake in one tenant role cannot mint a longer-lived token than the
# platform contract allows.
info "tuning token TTLs (default ${DEFAULT_TOKEN_TTL}, max ${MAX_TOKEN_TTL})"
vault_must POST "sys/auth/${AUTH_MOUNT}/tune" "$(jq -nc \
  --arg d "${DEFAULT_TOKEN_TTL}" \
  --arg m "${MAX_TOKEN_TTL}" \
  '{ default_lease_ttl: $d, max_lease_ttl: $m }')"

info "AppRole ready at '${AUTH_MOUNT}/'"
