#!/usr/bin/env bash
# Issue a working PostgreSQL credential with a bounded TTL.
set -euo pipefail

COMMON_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)"
# shellcheck source=../common/setup.sh
. "${COMMON_DIR}/setup.sh"
# shellcheck source=../common/database.sh
. "${COMMON_DIR}/database.sh"

credentials_require_env
conformance_login_tenants

ROLE_A_RO="tenant-${TENANT_A}-readonly"
ROLE_A_RW="tenant-${TENANT_A}-readwrite"

suite "Credential issuance"

CRED="$(issue_credential "${TOKEN_A_WRITER}" "${ROLE_A_RO}")" || {
  fail "tenant can request a dynamic credential" "HTTP ${ASSERT_STATUS} from ${DATABASE_MOUNT}/creds/${ROLE_A_RO}"
  finish
}
pass "tenant can request a dynamic credential"

IFS=$'\t' read -r CRED_USER CRED_PASS CRED_LEASE LEASE_TTL <<< "${CRED}"

assert_not_empty "${CRED_USER}"  "issued credential has a username"
assert_not_empty "${CRED_PASS}"  "issued credential has a password"
assert_not_empty "${CRED_LEASE}" "issued credential has a lease ID"

# The prefix is what makes orphan detection possible later. If Vault's username
# template ever changes, the residue scan silently stops finding anything, so
# the assumption is asserted here rather than left implicit.
assert_contains "${CRED_USER}" "v-" \
  "username carries the Vault prefix that orphan detection relies on"

suite "The credential actually works"

# Vault reporting success is not evidence the credential works. A wrong
# connection URL, a failed GRANT, or a password-encoding mismatch all produce a
# perfectly well-formed response and a credential that cannot log in.
assert_command_succeeds "issued credential can connect to PostgreSQL" \
  db_query_succeeds "${CRED_USER}" "${CRED_PASS}" "SELECT 1"

assert_eq "${CRED_USER}" \
  "$(db_query "${CRED_USER}" "${CRED_PASS}" "SELECT current_user;" | tr -d '[:space:]')" \
  "the connection authenticates as the issued user, not a shared one"

assert_command_succeeds "the role really exists in PostgreSQL" \
  db_role_exists "${CRED_USER}"

suite "Lifetime is bounded"

if [ "${LEASE_TTL}" -gt 0 ] 2>/dev/null; then
  pass "credential has a finite TTL (${LEASE_TTL}s)"
else
  # A credential with no expiry is a static credential with extra steps, and
  # defeats the entire reason for using dynamic secrets.
  fail "credential has a finite TTL" "lease_duration was '${LEASE_TTL}' - the credential does not expire"
fi

MAX_TTL_SECONDS="$(printf '%s' "${DATABASE_MAX_TTL}" | awk '
  /h$/ { gsub(/h/,""); print $0 * 3600; next }
  /m$/ { gsub(/m/,""); print $0 * 60;   next }
  /s$/ { gsub(/s/,""); print $0 + 0;    next }
              { print $0 + 0 }')"

if [ "${LEASE_TTL}" -le "${MAX_TTL_SECONDS}" ]; then
  pass "TTL is within the configured maximum (${DATABASE_MAX_TTL})"
else
  fail "TTL is within the configured maximum" "${LEASE_TTL}s exceeds ${MAX_TTL_SECONDS}s"
fi

suite "Each request yields a distinct credential"

CRED2="$(issue_credential "${TOKEN_A_WRITER}" "${ROLE_A_RO}")"
IFS=$'\t' read -r CRED2_USER CRED2_PASS CRED2_LEASE _ <<< "${CRED2}"

# Reuse would make credentials untraceable to a specific request and would mean
# revoking one consumer's access revokes everyone's.
if [ "${CRED_USER}" != "${CRED2_USER}" ]; then
  pass "a second request issues a different username"
else
  fail "a second request issues a different username" "both requests returned ${CRED_USER}"
fi

if [ "${CRED_PASS}" != "${CRED2_PASS}" ]; then
  pass "a second request issues a different password"
else
  fail "a second request issues a different password" "passwords are identical"
fi

suite "Both privilege tiers are issuable"

RW="$(issue_credential "${TOKEN_A_WRITER}" "${ROLE_A_RW}")" || {
  fail "tenant can request the readwrite role" "HTTP ${ASSERT_STATUS}"
  finish
}
pass "tenant can request the readwrite role"
IFS=$'\t' read -r RW_USER _ RW_LEASE _ <<< "${RW}"

revoke_lease "${TOKEN_A_WRITER}" "${CRED_LEASE}" || true
revoke_lease "${TOKEN_A_WRITER}" "${CRED2_LEASE}" || true
revoke_lease "${TOKEN_A_WRITER}" "${RW_LEASE}" || true
wait_until_db_role_gone "${CRED_USER}" 15 || true
wait_until_db_role_gone "${CRED2_USER}" 15 || true
wait_until_db_role_gone "${RW_USER}" 15 || true

finish
