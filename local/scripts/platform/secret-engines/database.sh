#!/usr/bin/env bash
# Mount the database secrets engine when ENABLE_DYNAMIC_CREDENTIALS=true.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib/common.sh
. "${SCRIPT_DIR}/../../lib/common.sh"
# shellcheck source=../../lib/vault.sh
. "${SCRIPT_DIR}/../../lib/vault.sh"

platform_defaults

if ! credentials_enabled; then
  info "dynamic credentials disabled - skipping database engine (isolation only)"
  exit 0
fi

vault_require_env

: "${DATABASE_CONNECTION_URL:?DATABASE_CONNECTION_URL must be supplied by the deployment target}"
: "${DATABASE_USERNAME:?DATABASE_USERNAME must be supplied by the deployment target}"
: "${DATABASE_PASSWORD:?DATABASE_PASSWORD must be supplied by the deployment target}"

DATABASE_CONNECTION_NAME="${DATABASE_CONNECTION_NAME:-app}"

if vault_mount_exists "${DATABASE_MOUNT}"; then
  info "database mount '${DATABASE_MOUNT}/' already exists"
else
  info "mounting database engine at '${DATABASE_MOUNT}/'"
  vault_must POST "sys/mounts/${DATABASE_MOUNT}" "$(jq -nc \
    --arg d "${DATABASE_DEFAULT_TTL}" \
    --arg m "${DATABASE_MAX_TTL}" \
    '{
       type: "database",
       description: "Dynamic database credentials",
       config: { default_lease_ttl: $d, max_lease_ttl: $m }
     }')"
fi

info "configuring connection '${DATABASE_CONNECTION_NAME}'"

# allowed_roles is the engine-level boundary. Restricting it to tenant-* means
# a role created outside the naming convention cannot use this connection at
# all - so the convention that makes per-tenant policy scoping possible is
# enforced by Vault rather than by reviewer discipline.
#
# max_open_connections caps Vault's share of PostgreSQL's connection budget.
# Left unbounded, credential issuance under load can exhaust max_connections
# and take down the application that is merely a neighbour here.
vault_must POST "${DATABASE_MOUNT}/config/${DATABASE_CONNECTION_NAME}" "$(jq -nc \
  --arg url "${DATABASE_CONNECTION_URL}" \
  --arg user "${DATABASE_USERNAME}" \
  --arg pass "${DATABASE_PASSWORD}" \
  --argjson maxopen "${DATABASE_MAX_OPEN_CONNECTIONS:-8}" \
  '{
     plugin_name: "postgresql-database-plugin",
     connection_url: $url,
     username: $user,
     password: $pass,
     allowed_roles: ["tenant-*"],
     max_open_connections: $maxopen,
     max_idle_connections: 2,
     max_connection_lifetime: "5m",
     verify_connection: true,
     password_authentication: "password"
   }')"

# The write above verifies the connection, so reaching this point means Vault
# authenticated to PostgreSQL successfully. Reported explicitly because the
# alternative - discovering it at first credential request - attributes a
# configuration error to whichever tenant happened to ask first.
info "database engine ready at '${DATABASE_MOUNT}/' (connection verified)"
