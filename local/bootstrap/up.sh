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
if [ "${WITH_CREDENTIALS}" = "true" ]; then
  ENABLE_DYNAMIC_CREDENTIALS=true
  export ENABLE_DYNAMIC_CREDENTIALS
fi
load_env
ensure_bootstrap_dir

if credentials_enabled; then
  compose up -d --wait vault postgres
else
  compose up -d --wait vault
fi

compose --profile bootstrap run --rm --no-deps bootstrap
info "VAULT_ADDR=${VAULT_ADDR}"
