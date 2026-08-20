#!/usr/bin/env bash
# ./setup.sh [--with-credentials]
# Local Shamir init/unseal. Not production KMS auto-unseal.
set -euo pipefail

# shellcheck source=bootstrap/lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/bootstrap/lib.sh"

WITH_CREDENTIALS=false
while [ $# -gt 0 ]; do
  case "$1" in
    --with-credentials) WITH_CREDENTIALS=true ;;
    -h|--help) sed -n '2,3p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
  shift
done

require_docker
require_cmd curl jq
load_env

if [ "${WITH_CREDENTIALS}" = "true" ]; then
  ENABLE_DYNAMIC_CREDENTIALS=true
  export ENABLE_DYNAMIC_CREDENTIALS
  awk '
    BEGIN { found = 0 }
    /^ENABLE_DYNAMIC_CREDENTIALS=/ { print "ENABLE_DYNAMIC_CREDENTIALS=true"; found = 1; next }
    { print }
    END { if (!found) print "ENABLE_DYNAMIC_CREDENTIALS=true" }
  ' "${ENV_FILE}" > "${ENV_FILE}.tmp" && mv "${ENV_FILE}.tmp" "${ENV_FILE}"
fi

info "local setup (Shamir unseal automation; not production KMS auto-unseal)"

if [ "${WITH_CREDENTIALS}" = "true" ]; then
  "${ADAPTER_DIR}/bootstrap/up.sh" --with-credentials
else
  "${ADAPTER_DIR}/bootstrap/up.sh"
fi

"${ADAPTER_DIR}/bootstrap/bootstrap.sh"

ensure_synthetic_tenant() {
  local id="$1" tok
  provisioning_token_usable || die "provisioning token is unusable after bootstrap"
  tok="$(cat "${BOOTSTRAP_DIR}/provisioning.token")"
  # Idempotent. Re-run so --with-credentials can add database roles later.
  VAULT_TOKEN="${tok}" "${REPO_ROOT}/scripts/tenants/create-tenant.sh" "${id}" --no-credentials
}

ensure_synthetic_tenant tenant-a
ensure_synthetic_tenant tenant-b

"${ADAPTER_DIR}/bootstrap/health.sh"

cat >&2 <<EOF

--------------------------------------------------------------------
Local Vault is ready.

  VAULT_ADDR   ${VAULT_ADDR}
  capability   isolation$(credentials_enabled && printf ' + dynamic database credentials')
  identities   ${BOOTSTRAP_DIR}/provisioning.token
               ${BOOTSTRAP_DIR}/operator.token
  unseal       ${INIT_FILE}   (gitignored, mode 600 — back this up)

  export VAULT_ADDR=${VAULT_ADDR}
  export VAULT_TOKEN=\$(cat ${BOOTSTRAP_DIR}/provisioning.token)

This is local Shamir unseal automation, not production KMS auto-unseal.
--------------------------------------------------------------------
EOF
