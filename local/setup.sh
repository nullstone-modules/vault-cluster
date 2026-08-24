#!/usr/bin/env bash
# ./setup.sh [--with-credentials]
# Local Shamir init/unseal via vault-utils one-shot. Not production KMS auto-unseal.
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

ensure_bootstrap_dir

info "starting Vault"
if [ "${WITH_CREDENTIALS}" = "true" ]; then
  compose --profile credentials up -d --wait vault postgres
else
  compose up -d --wait vault
fi

info "running vault-utils bootstrap (one-shot)"
compose --profile bootstrap run --rm --no-deps bootstrap

wait_for_unsealed 60 || die "Vault is still sealed after bootstrap"

"${ADAPTER_DIR}/bootstrap/health.sh"

cat >&2 <<EOF

--------------------------------------------------------------------
Local Vault is ready.

  VAULT_ADDR   ${VAULT_ADDR}
  capability   isolation$( [ "${WITH_CREDENTIALS}" = "true" ] && printf ' + dynamic database credentials')
  identities   ${BOOTSTRAP_DIR}/provisioning.token
               ${BOOTSTRAP_DIR}/operator.token
  unseal       ${INIT_FILE}   (gitignored, mode 600)

  export VAULT_ADDR=${VAULT_ADDR}
  export VAULT_TOKEN=\$(cat ${BOOTSTRAP_DIR}/provisioning.token)

  go test ./...
  go test ./internal/vaultcluster -run TestIsolation

This is local Shamir unseal automation, not production KMS auto-unseal.
--------------------------------------------------------------------
EOF
