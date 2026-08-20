#!/usr/bin/env bash
# create-tenant.sh <tenant-id> [--no-credentials]
# Prints role_id and secret_id once. Does not read tenant secrets.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/common.sh
. "${SCRIPT_DIR}/../lib/common.sh"
# shellcheck source=../lib/vault.sh
. "${SCRIPT_DIR}/../lib/vault.sh"
# shellcheck source=../validation/validate-tenant-id.sh
. "${SCRIPT_DIR}/../validation/validate-tenant-id.sh"

require_cmd curl jq sed
platform_defaults

ISSUE_CREDENTIALS=true
TENANT_ID=""

while [ $# -gt 0 ]; do
  case "$1" in
    --no-credentials) ISSUE_CREDENTIALS=false ;;
    -h|--help) sed -n '2,3p' "${BASH_SOURCE[0]}"; exit 0 ;;
    -*) die "unknown option: $1 (try --help)" ;;
    *)  [ -z "${TENANT_ID}" ] || die "only one tenant ID may be given"; TENANT_ID="$1" ;;
  esac
  shift
done

[ -n "${TENANT_ID}" ] || die "usage: $(basename "$0") <tenant-id> [--no-credentials]"

validate_tenant_id "${TENANT_ID}" || die "refusing to onboard an invalid tenant ID"

vault_require_env

READER_POLICY="$(tenant_policy_reader "${TENANT_ID}")"
WRITER_POLICY="$(tenant_policy_writer "${TENANT_ID}")"
DB_POLICY="$(tenant_policy_database "${TENANT_ID}")"
READER_ROLE="$(tenant_role_reader "${TENANT_ID}")"
WRITER_ROLE="$(tenant_role_writer "${TENANT_ID}")"

info "onboarding tenant '${TENANT_ID}'"
info "  subtree  $(tenant_kv_data_path "${TENANT_ID}")/"
info "  policies ${READER_POLICY}, ${WRITER_POLICY}$(credentials_enabled && printf ', %s' "${DB_POLICY}")"

# 1. Render, lint, apply. Lint before apply so a bad policy never goes live.
render_lint_apply() {
  local template="$1" policy_name="$2" rendered
  rendered="$("${SCRIPT_DIR}/render-policy.sh" "${template}" "${TENANT_ID}" "${policy_name}" | tail -n 1)"
  "${TESTS_DIR}/lint/policy-lint.sh" "${rendered}" \
    || die "rendered policy '${policy_name}' failed lint - NOT applied"
  vault_write_policy "${policy_name}" "${rendered}"
  info "  applied policy ${policy_name}"
}

render_lint_apply tenant-reader "${READER_POLICY}"
render_lint_apply tenant-writer "${WRITER_POLICY}"
credentials_enabled && render_lint_apply tenant-database "${DB_POLICY}"

# 2. AppRole roles. Writer also gets the database policy when credentials are on.
create_role() {
  local role="$1"; shift
  local policies_json; policies_json="$(printf '%s\n' "$@" | jq -R . | jq -sc .)"

  vault_must POST "auth/${AUTH_MOUNT}/role/${role}" "$(jq -nc \
    --argjson policies "${policies_json}" \
    --arg ttl "${DEFAULT_TOKEN_TTL}" \
    --arg max "${MAX_TOKEN_TTL}" \
    '{
       token_policies: $policies,
       token_ttl: $ttl,
       token_max_ttl: $max,
       token_type: "service",
       secret_id_ttl: "24h",
       secret_id_num_uses: 0,
       bind_secret_id: true
     }')"
  info "  created role ${role} -> [$*]"
}

create_role "${READER_ROLE}" "${READER_POLICY}"
if credentials_enabled; then
  create_role "${WRITER_ROLE}" "${WRITER_POLICY}" "${DB_POLICY}"
else
  create_role "${WRITER_ROLE}" "${WRITER_POLICY}"
fi

# 3. Database roles. Grants live in PostgreSQL; Vault only issues membership.
create_database_role() {
  local suffix="$1" group_role="$2"
  local role="tenant-${TENANT_ID}-${suffix}"

  vault_must POST "${DATABASE_MOUNT}/roles/${role}" "$(jq -nc \
    --arg db "${DATABASE_CONNECTION_NAME:-app}" \
    --arg grp "${group_role}" \
    --arg ttl "${DATABASE_DEFAULT_TTL}" \
    --arg max "${DATABASE_MAX_TTL}" \
    '{
       db_name: $db,
       creation_statements: [
         "CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '"'"'{{password}}'"'"' VALID UNTIL '"'"'{{expiration}}'"'"';",
         ("GRANT " + $grp + " TO \"{{name}}\";")
       ],
       default_ttl: $ttl,
       max_ttl: $max
     }')"
  info "  created database role ${role} (grants ${group_role})"
}

if credentials_enabled; then
  create_database_role readonly  app_readonly
  create_database_role readwrite app_readwrite
fi

# 4. Issue credentials to stdout once. Not written to disk.
if [ "${ISSUE_CREDENTIALS}" != "true" ]; then
  info "tenant '${TENANT_ID}' onboarded (no credentials issued)"
  exit 0
fi

issue_approle_credentials() {
  local role="$1" role_id secret_id

  vault_must GET "auth/${AUTH_MOUNT}/role/${role}/role-id"
  role_id="$(vault_body | jq -r '.data.role_id')"

  vault_must POST "auth/${AUTH_MOUNT}/role/${role}/secret-id" '{}'
  secret_id="$(vault_body | jq -r '.data.secret_id')"

  printf '%s\n' "----------------------------------------------------------"
  printf 'role       %s\n' "${role}"
  printf 'role_id    %s\n' "${role_id}"
  printf 'secret_id  %s\n' "${secret_id}"
}

printf '\n'
printf 'Credentials for tenant %s - shown once, not stored:\n\n' "${TENANT_ID}"
issue_approle_credentials "${READER_ROLE}"
issue_approle_credentials "${WRITER_ROLE}"
printf '%s\n\n' "----------------------------------------------------------"

info "tenant '${TENANT_ID}' onboarded"
