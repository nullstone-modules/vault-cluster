#!/usr/bin/env bash
# ./setup.sh [--with-credentials]
set -euo pipefail

# shellcheck source=lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

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
if credentials_enabled; then
  compose up -d --wait vault postgres
else
  compose up -d --wait vault
fi

info "running vault-utils bootstrap"
compose run --build --rm --no-deps bootstrap

wait_for_unsealed 60 || die "Vault is still sealed after bootstrap"

cat >&2 <<EOF

--------------------------------------------------------------------
Local Vault is ready.

  VAULT_ADDR   ${VAULT_ADDR}
  identities   ${BOOTSTRAP_DIR}/provisioning.token
               ${BOOTSTRAP_DIR}/operator.token
  unseal       ${INIT_FILE}

  export VAULT_ADDR=${VAULT_ADDR}
  export VAULT_TOKEN=\$(cat ${BOOTSTRAP_DIR}/provisioning.token)
--------------------------------------------------------------------
EOF
