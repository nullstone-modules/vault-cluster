#!/usr/bin/env bash
# PostgreSQL helpers for the dynamic-credential conformance tests.
#
# Sourced, not executed.
#
# Verifying a dynamic credential requires actually connecting with it. Vault
# reporting that it issued a credential is not evidence that the credential
# works, that it carries the intended privileges, or that revoking it takes
# effect - and each of those has failed independently in real deployments.
#
# The runtime coupling is confined to two variables. PSQL_CMD is how SQL gets
# executed and DB_TEST_HOST is where PostgreSQL is reachable from wherever that
# command runs; everything else here is generic.
#
# Environment:
#   PSQL_CMD        command that behaves like psql (default: psql)
#   DB_TEST_HOST    host PostgreSQL is reachable at (default: 127.0.0.1)
#   DB_TEST_PORT    port (default: 5432)
#   DB_TEST_NAME    database (default: appdb)
#   DB_ADMIN_USER   privileged user for residue scans (default: postgres)
#   DB_ADMIN_PASSWORD

# shellcheck shell=bash

PSQL_CMD="${PSQL_CMD:-psql}"
DB_TEST_HOST="${DB_TEST_HOST:-127.0.0.1}"
DB_TEST_PORT="${DB_TEST_PORT:-5432}"
DB_TEST_NAME="${DB_TEST_NAME:-appdb}"
DB_ADMIN_USER="${DB_ADMIN_USER:-postgres}"
DB_ADMIN_PASSWORD="${DB_ADMIN_PASSWORD:-}"

db_uri() {
  local user="$1" pass="$2"
  printf 'postgresql://%s:%s@%s:%s/%s?sslmode=disable' \
    "${user}" "${pass}" "${DB_TEST_HOST}" "${DB_TEST_PORT}" "${DB_TEST_NAME}"
}

# db_query USER PASSWORD SQL
# Prints the result unaligned and untupled, so callers can compare directly.
db_query() {
  local user="$1" pass="$2" sql="$3"
  # shellcheck disable=SC2086
  ${PSQL_CMD} "$(db_uri "${user}" "${pass}")" -v ON_ERROR_STOP=1 -tAc "${sql}" 2>&1
}

db_query_succeeds() {
  local user="$1" pass="$2" sql="$3"
  # shellcheck disable=SC2086
  ${PSQL_CMD} "$(db_uri "${user}" "${pass}")" -v ON_ERROR_STOP=1 -tAc "${sql}" >/dev/null 2>&1
}

db_admin_query() {
  db_query "${DB_ADMIN_USER}" "${DB_ADMIN_PASSWORD}" "$1"
}

db_available() {
  db_query_succeeds "${DB_ADMIN_USER}" "${DB_ADMIN_PASSWORD}" "SELECT 1"
}

credentials_require_env() {
  conformance_require_env
  credentials_enabled || die \
    "ENABLE_DYNAMIC_CREDENTIALS is not true.
     Running the credentials suite against a platform with the capability
     disabled would report a pass for tests that never executed, so this is a
     hard error rather than a skip."

  db_available || die \
    "cannot reach PostgreSQL as ${DB_ADMIN_USER}@${DB_TEST_HOST}:${DB_TEST_PORT}/${DB_TEST_NAME}
     using PSQL_CMD='${PSQL_CMD}'.
     These tests must connect with the credentials Vault issues; without a
     working connection they could only assert that Vault returned a string."
}

# --- Dynamic credentials -----------------------------------------------------

# issue_credential TOKEN ROLE
# Prints "username<TAB>password<TAB>lease_id<TAB>lease_duration" and returns nonzero on failure.
issue_credential() {
  local token="$1" role="$2"
  as_token "${token}" GET "${DATABASE_MOUNT}/creds/${role}"
  case "${ASSERT_STATUS}" in
    2*) ;;
    *) return 1 ;;
  esac
  printf '%s\t%s\t%s\t%s' \
    "$(printf '%s' "${ASSERT_BODY}" | jq -r '.data.username')" \
    "$(printf '%s' "${ASSERT_BODY}" | jq -r '.data.password')" \
    "$(printf '%s' "${ASSERT_BODY}" | jq -r '.lease_id')" \
    "$(printf '%s' "${ASSERT_BODY}" | jq -r '.lease_duration // 0')"
}

# Tenant database policy can revoke its own lease; provisioning cannot.
# Callers must read four fields (user, password, lease_id, ttl). Bash assigns
# leftover columns to the last variable, so a 3-variable read corrupts lease_id.
revoke_lease() {
  local token="$1" lease_id="$2"
  [ -n "${lease_id}" ] && [ "${lease_id}" != "null" ] || return 1
  as_token "${token}" PUT "sys/leases/revoke" \
    "$(jq -nc --arg l "${lease_id}" '{lease_id: $l}')"
  case "${ASSERT_STATUS}" in
    2*) return 0 ;;
    *) return 1 ;;
  esac
}

wait_until_db_role_gone() {
  local user="$1" attempts="${2:-15}"
  local _
  for _ in $(seq 1 "${attempts}"); do
    db_role_exists "${user}" || return 0
    sleep 1
  done
  return 1
}

# Vault's PostgreSQL plugin prefixes every generated username with "v-", which
# is what makes orphan detection possible: any v-* role with no corresponding
# lease is residue.
VAULT_ROLE_PREFIX="v-"

count_vault_db_roles() {
  db_admin_query "SELECT count(*) FROM pg_roles WHERE rolname LIKE '${VAULT_ROLE_PREFIX}%';" \
    | tr -d '[:space:]'
}

list_vault_db_roles() {
  db_admin_query "SELECT rolname FROM pg_roles WHERE rolname LIKE '${VAULT_ROLE_PREFIX}%' ORDER BY rolname;"
}

db_role_exists() {
  local rolname="$1" result
  result="$(db_admin_query "SELECT 1 FROM pg_roles WHERE rolname = '${rolname}';" | tr -d '[:space:]')"
  [ "${result}" = "1" ]
}
