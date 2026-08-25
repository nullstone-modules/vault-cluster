#!/usr/bin/env bash
# Stop containers. Named volumes are kept.
set -euo pipefail

# shellcheck source=lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

case "${1:-}" in
  -h|--help) sed -n '2,2p' "${BASH_SOURCE[0]}"; exit 0 ;;
  "") ;;
  *) die "unknown argument: $1 (did you mean reset.sh?)" ;;
esac

require_docker
load_env

# Both profiles are torn down regardless of the current credentials setting.
# Otherwise disabling credentials in .env would strand a running PostgreSQL
# container that nothing subsequently manages.
info "stopping services (volumes preserved)"
docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" --profile credentials down --remove-orphans

info "stopped. Data volumes are intact; Vault will be sealed on next start."
info "run ./setup.sh to start and unseal. To destroy data, ./reset.sh --yes"
