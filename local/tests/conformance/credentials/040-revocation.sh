#!/usr/bin/env bash
# Explicit revoke and TTL expiry stop the credential and drop the DB role.
set -euo pipefail

COMMON_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)"
# shellcheck source=../common/setup.sh
. "${COMMON_DIR}/setup.sh"
# shellcheck source=../common/database.sh
. "${COMMON_DIR}/database.sh"

credentials_require_env
conformance_login_tenants

PROVISIONING_TOKEN="${VAULT_TOKEN}"

# Explicit revocation
suite "Explicit revocation"

CRED="$(issue_credential "${TOKEN_A_WRITER}" "tenant-${TENANT_A}-readonly")" \
  || die "could not issue a credential to revoke"
IFS=$'\t' read -r CRED_USER CRED_PASS CRED_LEASE _ <<< "${CRED}"

assert_command_succeeds "credential works before revocation" \
  db_query_succeeds "${CRED_USER}" "${CRED_PASS}" "SELECT 1"

as_token "${TOKEN_A_WRITER}" PUT "sys/leases/revoke" \
  "$(jq -nc --arg l "${CRED_LEASE}" '{lease_id: $l}')"
case "${ASSERT_STATUS}" in
  2*) pass "tenant can revoke its own lease" ;;
  *)  fail "tenant can revoke its own lease" "HTTP ${ASSERT_STATUS}" ;;
esac

# Revocation is asynchronous in Vault. A short bounded wait, then assert -
# rather than a fixed sleep long enough to always pass, which would hide a
# revocation path that is merely very slow.
revoked=false
for _ in 1 2 3 4 5 6 7 8 9 10; do
  db_query_succeeds "${CRED_USER}" "${CRED_PASS}" "SELECT 1" || { revoked=true; break; }
  sleep 1
done

if [ "${revoked}" = "true" ]; then
  pass "revoked credential can no longer connect"
else
  fail "revoked credential can no longer connect" \
       "user ${CRED_USER} still authenticates 10s after revocation - access outlives revocation"
fi

if db_role_exists "${CRED_USER}"; then
  fail "revoked credential's database role is dropped" \
       "role ${CRED_USER} still exists in pg_roles - this is an orphan"
else
  pass "revoked credential's database role is dropped"
fi

# TTL expiry
suite "TTL expiry"

# A dedicated short-TTL role: waiting out the platform default of an hour is
# not viable in CI, and shortening the default just for tests would mean the
# tested configuration is not the shipped one.
EXPIRY_ROLE="tenant-${TENANT_A}-ttltest"
VAULT_TOKEN="${PROVISIONING_TOKEN}"

vault_request POST "${DATABASE_MOUNT}/roles/${EXPIRY_ROLE}" "$(jq -nc \
  --arg db "${DATABASE_CONNECTION_NAME:-app}" \
  '{
     db_name: $db,
     creation_statements: [
       "CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '"'"'{{password}}'"'"' VALID UNTIL '"'"'{{expiration}}'"'"';",
       "GRANT app_readonly TO \"{{name}}\";"
     ],
     default_ttl: "10s",
     max_ttl: "20s"
   }')" >/dev/null 2>&1

if [ "${VAULT_STATUS}" != "204" ] && [ "${VAULT_STATUS}" != "200" ]; then
  fail "short-TTL test role created" "HTTP ${VAULT_STATUS} - cannot test expiry"
  finish
fi
pass "short-TTL test role created"

EXP_CRED="$(issue_credential "${TOKEN_A_WRITER}" "${EXPIRY_ROLE}")" || {
  fail "short-TTL credential issued" "HTTP ${ASSERT_STATUS}"
  finish
}
IFS=$'\t' read -r EXP_USER EXP_PASS _ _ <<< "${EXP_CRED}"

assert_command_succeeds "short-TTL credential works immediately after issue" \
  db_query_succeeds "${EXP_USER}" "${EXP_PASS}" "SELECT 1"

printf '    ....  waiting for the 10s TTL to elapse\n'
expired=false
for _ in $(seq 1 30); do
  sleep 1
  db_query_succeeds "${EXP_USER}" "${EXP_PASS}" "SELECT 1" || { expired=true; break; }
done

if [ "${expired}" = "true" ]; then
  pass "credential stops working after its TTL expires"
else
  fail "credential stops working after its TTL expires" \
       "user ${EXP_USER} still authenticates 30s after a 10s TTL - credentials do not expire"
fi

if db_role_exists "${EXP_USER}"; then
  fail "expired credential's database role is dropped" \
       "role ${EXP_USER} remains in pg_roles after expiry - orphaned"
else
  pass "expired credential's database role is dropped"
fi

# Revoking one credential does not affect another
suite "Revocation is scoped to a single lease"

KEEP="$(issue_credential "${TOKEN_A_WRITER}" "tenant-${TENANT_A}-readonly")"
IFS=$'\t' read -r KEEP_USER KEEP_PASS KEEP_LEASE _ <<< "${KEEP}"

DROP="$(issue_credential "${TOKEN_A_WRITER}" "tenant-${TENANT_A}-readonly")"
IFS=$'\t' read -r DROP_USER DROP_PASS DROP_LEASE _ <<< "${DROP}"

as_token "${TOKEN_A_WRITER}" PUT "sys/leases/revoke" \
  "$(jq -nc --arg l "${DROP_LEASE}" '{lease_id: $l}')"
sleep 2

# Over-broad revocation is as damaging as failed revocation: revoking one
# application's credential must not take down every other consumer of the same
# role.
assert_command_succeeds "an unrelated credential still works after another is revoked" \
  db_query_succeeds "${KEEP_USER}" "${KEEP_PASS}" "SELECT 1"

assert_command_fails "the revoked credential is the one that stopped working" \
  db_query_succeeds "${DROP_USER}" "${DROP_PASS}" "SELECT 1"

revoke_lease "${TOKEN_A_WRITER}" "${KEEP_LEASE}" || true
wait_until_db_role_gone "${KEEP_USER}" 15 || true

VAULT_TOKEN="${PROVISIONING_TOKEN}"
vault_request DELETE "${DATABASE_MOUNT}/roles/${EXPIRY_ROLE}" >/dev/null 2>&1 || true

finish
