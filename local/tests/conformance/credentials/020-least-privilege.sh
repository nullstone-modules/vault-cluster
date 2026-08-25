#!/usr/bin/env bash
# Readonly SELECT only. Readwrite DML, not DDL/superuser.
set -euo pipefail

COMMON_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)"
# shellcheck source=../common/setup.sh
. "${COMMON_DIR}/setup.sh"
# shellcheck source=../common/database.sh
. "${COMMON_DIR}/database.sh"

credentials_require_env
conformance_login_tenants

RO="$(issue_credential "${TOKEN_A_WRITER}" "tenant-${TENANT_A}-readonly")" \
  || die "could not issue a readonly credential"
IFS=$'\t' read -r RO_USER RO_PASS RO_LEASE _ <<< "${RO}"

RW="$(issue_credential "${TOKEN_A_WRITER}" "tenant-${TENANT_A}-readwrite")" \
  || die "could not issue a readwrite credential"
IFS=$'\t' read -r RW_USER RW_PASS RW_LEASE _ <<< "${RW}"

suite "Readonly credential can read"

assert_command_succeeds "readonly can SELECT from the application schema" \
  db_query_succeeds "${RO_USER}" "${RO_PASS}" "SELECT count(*) FROM app.customers;"

# Confirms real rows come back rather than an empty result that an
# over-restrictive grant would also produce without erroring.
ROW_COUNT="$(db_query "${RO_USER}" "${RO_PASS}" "SELECT count(*) FROM app.customers;" | tr -d '[:space:]')"
if [ "${ROW_COUNT}" -ge 1 ] 2>/dev/null; then
  pass "readonly actually returns rows (${ROW_COUNT})"
else
  fail "readonly actually returns rows" "got '${ROW_COUNT}' - SELECT succeeded but returned nothing"
fi

suite "Readonly credential cannot write"

assert_command_fails "readonly cannot INSERT" \
  db_query_succeeds "${RO_USER}" "${RO_PASS}" \
  "INSERT INTO app.customers (name) VALUES ('should-not-exist');"

assert_command_fails "readonly cannot UPDATE" \
  db_query_succeeds "${RO_USER}" "${RO_PASS}" \
  "UPDATE app.customers SET name = 'modified' WHERE id = 1;"

assert_command_fails "readonly cannot DELETE" \
  db_query_succeeds "${RO_USER}" "${RO_PASS}" \
  "DELETE FROM app.customers WHERE id = 1;"

assert_command_fails "readonly cannot CREATE TABLE" \
  db_query_succeeds "${RO_USER}" "${RO_PASS}" \
  "CREATE TABLE app.should_not_exist (id int);"

assert_command_fails "readonly cannot DROP TABLE" \
  db_query_succeeds "${RO_USER}" "${RO_PASS}" "DROP TABLE app.orders;"

suite "Readwrite credential can write, within limits"

assert_command_succeeds "readwrite can INSERT" \
  db_query_succeeds "${RW_USER}" "${RW_PASS}" \
  "INSERT INTO app.customers (name) VALUES ('FAKE-conformance-row');"

assert_command_succeeds "readwrite can DELETE its own row" \
  db_query_succeeds "${RW_USER}" "${RW_PASS}" \
  "DELETE FROM app.customers WHERE name = 'FAKE-conformance-row';"

# Data access is not schema authority. A credential that can add or drop tables
# can change the application's contract, which is a different privilege from
# changing its rows.
assert_command_fails "readwrite cannot CREATE TABLE" \
  db_query_succeeds "${RW_USER}" "${RW_PASS}" \
  "CREATE TABLE app.should_not_exist (id int);"

suite "Neither credential is privileged"

for pair in "readonly:${RO_USER}:${RO_PASS}" "readwrite:${RW_USER}:${RW_PASS}"; do
  label="${pair%%:*}"; rest="${pair#*:}"; user="${rest%%:*}"; pass="${rest#*:}"

  IS_SUPER="$(db_admin_query "SELECT rolsuper FROM pg_roles WHERE rolname = '${user}';" | tr -d '[:space:]')"
  assert_eq "f" "${IS_SUPER}" "${label} credential is not a superuser"

  CAN_CREATE_ROLE="$(db_admin_query "SELECT rolcreaterole FROM pg_roles WHERE rolname = '${user}';" | tr -d '[:space:]')"
  # CREATEROLE would let a dynamic credential mint a permanent user and outlive
  # its own TTL entirely - the single change that would defeat expiry.
  assert_eq "f" "${CAN_CREATE_ROLE}" "${label} credential cannot create roles"

  CAN_CREATE_DB="$(db_admin_query "SELECT rolcreatedb FROM pg_roles WHERE rolname = '${user}';" | tr -d '[:space:]')"
  assert_eq "f" "${CAN_CREATE_DB}" "${label} credential cannot create databases"

  assert_command_fails "${label} cannot read pg_shadow (password hashes)" \
    db_query_succeeds "${user}" "${pass}" "SELECT * FROM pg_shadow;"
done

suite "Credentials expire"

for pair in "readonly:${RO_USER}" "readwrite:${RW_USER}"; do
  label="${pair%%:*}"; user="${pair#*:}"
  VALID_UNTIL="$(db_admin_query "SELECT rolvaliduntil FROM pg_roles WHERE rolname = '${user}';" | tr -d '[:space:]')"
  # Belt and braces alongside Vault's lease: even if lease revocation failed
  # entirely, PostgreSQL itself would refuse the login after this timestamp.
  assert_not_empty "${VALID_UNTIL}" "${label} credential has a VALID UNTIL expiry in PostgreSQL"
done

revoke_lease "${TOKEN_A_WRITER}" "${RO_LEASE}" || true
revoke_lease "${TOKEN_A_WRITER}" "${RW_LEASE}" || true
wait_until_db_role_gone "${RO_USER}" 15 || true
wait_until_db_role_gone "${RW_USER}" 15 || true

finish
