#!/usr/bin/env bash
# One tenant cannot request another tenant's database credentials.
set -euo pipefail

COMMON_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)"
# shellcheck source=../common/setup.sh
. "${COMMON_DIR}/setup.sh"
# shellcheck source=../common/database.sh
. "${COMMON_DIR}/database.sh"

credentials_require_env
conformance_login_tenants

suite "A tenant can obtain its own credentials (control)"

CRED_A="$(issue_credential "${TOKEN_A_WRITER}" "tenant-${TENANT_A}-readonly")" || {
  fail "${TENANT_A} can request its own readonly credential" "HTTP ${ASSERT_STATUS}"
  finish
}
pass "${TENANT_A} can request its own readonly credential"
IFS=$'\t' read -r USER_A _ LEASE_A _ <<< "${CRED_A}"

CRED_B="$(issue_credential "${TOKEN_B_WRITER}" "tenant-${TENANT_B}-readonly")" || {
  fail "${TENANT_B} can request its own readonly credential" "HTTP ${ASSERT_STATUS}"
  finish
}
pass "${TENANT_B} can request its own readonly credential"
IFS=$'\t' read -r USER_B _ LEASE_B _ <<< "${CRED_B}"

suite "Cross-tenant credential requests are denied"

assert_denied "${TOKEN_A_WRITER}" GET "${DATABASE_MOUNT}/creds/tenant-${TENANT_B}-readonly" "" \
  "${TENANT_A} cannot request ${TENANT_B}'s readonly credential"

assert_denied "${TOKEN_A_WRITER}" GET "${DATABASE_MOUNT}/creds/tenant-${TENANT_B}-readwrite" "" \
  "${TENANT_A} cannot request ${TENANT_B}'s readwrite credential"

assert_denied "${TOKEN_B_WRITER}" GET "${DATABASE_MOUNT}/creds/tenant-${TENANT_A}-readwrite" "" \
  "${TENANT_B} cannot request ${TENANT_A}'s readwrite credential"

assert_denied "${TOKEN_A_READER}" GET "${DATABASE_MOUNT}/creds/tenant-${TENANT_B}-readonly" "" \
  "${TENANT_A}'s reader identity cannot request ${TENANT_B}'s credential"

suite "Engine administration is denied to tenants"

# Creating a role is how a tenant would grant itself access to anything the
# vault_admin database user can reach, bypassing the per-tenant role boundary
# entirely.
assert_denied "${TOKEN_A_WRITER}" POST "${DATABASE_MOUNT}/roles/tenant-rogue" \
  '{"db_name":"app","creation_statements":["CREATE ROLE \"{{name}}\" SUPERUSER LOGIN PASSWORD '"'"'{{password}}'"'"';"]}' \
  "a tenant cannot define its own database role"

assert_denied "${TOKEN_A_WRITER}" POST \
  "${DATABASE_MOUNT}/roles/tenant-${TENANT_A}-readonly" \
  '{"db_name":"app","creation_statements":["CREATE ROLE \"{{name}}\" SUPERUSER LOGIN PASSWORD '"'"'{{password}}'"'"';"]}' \
  "a tenant cannot redefine its own role to escalate privileges"

assert_denied "${TOKEN_A_WRITER}" GET "${DATABASE_MOUNT}/config/app" "" \
  "a tenant cannot read the engine's connection configuration"

assert_denied "${TOKEN_A_WRITER}" POST "${DATABASE_MOUNT}/rotate-root/app" '{}' \
  "a tenant cannot rotate the engine's root credential"

assert_denied "${TOKEN_A_WRITER}" GET "${DATABASE_MOUNT}/roles?list=true" "" \
  "a tenant cannot enumerate all database roles"

suite "Reader identities cannot obtain write credentials"

# The reader AppRole deliberately does not carry the database policy. A
# read-only tenant identity able to mint a readwrite database credential would
# make the reader/writer distinction cosmetic.
assert_denied "${TOKEN_A_READER}" GET "${DATABASE_MOUNT}/creds/tenant-${TENANT_A}-readwrite" "" \
  "${TENANT_A}'s reader identity cannot request a readwrite credential"

suite "Isolation is unchanged by dynamic credentials"

# Re-asserted here because enabling the database engine adds policies to the
# writer role. Additive changes are exactly where a widened deny matrix would
# go unnoticed - the isolation suite runs without the database engine and so
# would never see it.
conformance_seed_fixtures

assert_denied "${TOKEN_A_READER}" GET "$(kv_data "${TENANT_B}" "${FIXTURE_SECRET_NAME}")" "" \
  "KV cross-tenant read is still denied with credentials enabled"

assert_denied "${TOKEN_A_READER}" GET "${KV_MOUNT}/metadata/${TENANT_PREFIX}?list=true" "" \
  "tenant enumeration is still denied with credentials enabled"

assert_allowed "${TOKEN_A_READER}" GET "$(kv_data "${TENANT_A}" "${FIXTURE_SECRET_NAME}")" "" \
  "same-tenant KV read still works with credentials enabled"

revoke_lease "${TOKEN_A_WRITER}" "${LEASE_A}" || true
revoke_lease "${TOKEN_B_WRITER}" "${LEASE_B}" || true
wait_until_db_role_gone "${USER_A}" 15 || true
wait_until_db_role_gone "${USER_B}" 15 || true

finish
