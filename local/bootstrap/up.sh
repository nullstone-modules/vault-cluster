#!/usr/bin/env bash
# ./up.sh [--with-credentials]
set -euo pipefail

# shellcheck source=lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

WITH_CREDENTIALS=false
while [ $# -gt 0 ]; do
  case "$1" in
    --with-credentials) WITH_CREDENTIALS=true ;;
    -h|--help) sed -n '2,2p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
  shift
done

require_docker
require_cmd curl jq
load_env

# A command-line flag is a per-run override of the .env default, so enabling
# dynamic credentials for one run does not silently become the new default.
if [ "${WITH_CREDENTIALS}" = "true" ]; then
  ENABLE_DYNAMIC_CREDENTIALS=true
  export ENABLE_DYNAMIC_CREDENTIALS
fi

if credentials_enabled; then
  info "capabilities: isolation + dynamic database credentials"
else
  info "capability: isolation only - PostgreSQL will not be started"
fi

info "pulling pinned images (digest-pinned, so this is a no-op once cached)"
compose pull --quiet 2>/dev/null || warn "pull failed or offline - using locally cached images"

# Bind-mount target must exist before Compose creates it as root.
ensure_bootstrap_dir

info "starting services (Vault + Shamir unseal sidecar)"
compose up -d --wait

wait_for_vault 60
wait_for_unsealed 90

info "VAULT_ADDR=${VAULT_ADDR}"
