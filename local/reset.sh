#!/usr/bin/env bash
# ./reset.sh --yes  — destroy volumes and unseal keys.
set -euo pipefail

# shellcheck source=bootstrap/lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/bootstrap/lib.sh"

CONFIRMED=false
KEEP_BOOTSTRAP=false
while [ $# -gt 0 ]; do
  case "$1" in
    --yes) CONFIRMED=true ;;
    --keep-bootstrap) KEEP_BOOTSTRAP=true ;;
    -h|--help) sed -n '2,2p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
  shift
done

if [ "${CONFIRMED}" != "true" ]; then
  cat >&2 <<'EOF'
Refusing to run without explicit confirmation.

This DESTROYS all local Vault data, audit logs, PostgreSQL data, and the
unseal keys. It cannot be undone.

Re-run with:  ./reset.sh --yes
EOF
  exit 1
fi

require_docker
load_env

info "destroying containers and volumes"
docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" --profile credentials down \
  --volumes --remove-orphans

if [ "${KEEP_BOOTSTRAP}" = "true" ]; then
  warn "keeping ${BOOTSTRAP_DIR} - note these keys no longer unseal anything"
elif [ -d "${BOOTSTRAP_DIR}" ]; then
  # Stale unseal shares are worse than no shares: they invite an operator to
  # believe recovery is possible when the data they unlock no longer exists.
  info "removing stale bootstrap material"
  rm -rf "${BOOTSTRAP_DIR}"
fi

info "reset complete. Run ./setup.sh to start fresh."
