#!/usr/bin/env bash
# Conformance test setup: configuration, tenant login, and fixtures.
#
# Sourced, not executed.
#
# Reads only the environment variables named in the platform contract, so the
# same suite runs unchanged against any conforming Vault.

# shellcheck shell=bash

CONFORMANCE_COMMON_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PLATFORM_SCRIPTS="$(cd -- "${CONFORMANCE_COMMON_DIR}/../../../scripts" && pwd)"

# shellcheck source=../../../scripts/lib/common.sh
. "${PLATFORM_SCRIPTS}/lib/common.sh"
# shellcheck source=../../../scripts/lib/vault.sh
. "${PLATFORM_SCRIPTS}/lib/vault.sh"
# shellcheck source=assert.sh
. "${CONFORMANCE_COMMON_DIR}/assert.sh"

platform_defaults
require_cmd curl jq

TENANT_A="${TENANT_A:-tenant-a}"
TENANT_B="${TENANT_B:-tenant-b}"

conformance_require_env() {
  vault_require_env
  [ -n "${TENANT_A}" ] || die "TENANT_A is not set"
  [ -n "${TENANT_B}" ] || die "TENANT_B is not set"
}

# --- Tenant authentication ----------------------------------------------------
#
# The suite logs in as each tenant rather than being handed tokens, so it
# exercises the real credential path: role binding, secret-id issuance, and
# login. Tokens supplied from outside would skip exactly the part of the
# identity model most likely to be misconfigured.

approle_login() {
  local role="$1" role_id secret_id token

  vault_request GET "auth/${AUTH_MOUNT}/role/${role}/role-id" >/dev/null 2>&1 \
    || die "cannot read role-id for '${role}' (HTTP ${VAULT_STATUS}) - has the tenant been onboarded?"
  role_id="$(vault_body | jq -r '.data.role_id')"

  vault_request POST "auth/${AUTH_MOUNT}/role/${role}/secret-id" '{}' >/dev/null 2>&1 \
    || die "cannot issue secret-id for '${role}' (HTTP ${VAULT_STATUS})"
  secret_id="$(vault_body | jq -r '.data.secret_id')"

  # Login is unauthenticated, so the current token must not leak into it.
  local saved="${VAULT_TOKEN}"
  VAULT_TOKEN=""
  vault_request POST "auth/${AUTH_MOUNT}/login" \
    "$(jq -nc --arg r "${role_id}" --arg s "${secret_id}" '{role_id: $r, secret_id: $s}')" >/dev/null 2>&1 \
    || { VAULT_TOKEN="${saved}"; die "AppRole login failed for '${role}' (HTTP ${VAULT_STATUS})"; }
  token="$(vault_body | jq -r '.auth.client_token')"
  VAULT_TOKEN="${saved}"

  [ -n "${token}" ] && [ "${token}" != "null" ] || die "login for '${role}' returned no token"
  printf '%s' "${token}"
}

# Populates TOKEN_A_READER, TOKEN_A_WRITER, TOKEN_B_READER, TOKEN_B_WRITER.
conformance_login_tenants() {
  TOKEN_A_READER="$(approle_login "$(tenant_role_reader "${TENANT_A}")")"
  TOKEN_A_WRITER="$(approle_login "$(tenant_role_writer "${TENANT_A}")")"
  TOKEN_B_READER="$(approle_login "$(tenant_role_reader "${TENANT_B}")")"
  TOKEN_B_WRITER="$(approle_login "$(tenant_role_writer "${TENANT_B}")")"
  export TOKEN_A_READER TOKEN_A_WRITER TOKEN_B_READER TOKEN_B_WRITER
}

# --- Path helpers -------------------------------------------------------------

kv_data() { printf '%s/data/%s/%s/%s' "${KV_MOUNT}" "${TENANT_PREFIX}" "$1" "${2:-}"; }
kv_meta() { printf '%s/metadata/%s/%s/%s' "${KV_MOUNT}" "${TENANT_PREFIX}" "$1" "${2:-}"; }

# KV v2 wraps written values in a "data" envelope. Getting this wrong stores a
# literal {"data":...} nested one level too deep, which reads back fine and
# fails only when an application looks for its key.
kv_payload() {
  jq -nc --arg k "$1" --arg v "$2" '{data: {($k): $v}}'
}

# --- Fixtures -----------------------------------------------------------------
#
# Fake values only, and clearly labelled as such. A test fixture that looks
# like a plausible credential eventually gets copied somewhere real.
FIXTURE_SECRET_NAME="${FIXTURE_SECRET_NAME:-app-config}"
FIXTURE_SECRET_KEY="api_key"
FIXTURE_VALUE_A="FAKE-not-a-real-credential-tenant-a"
FIXTURE_VALUE_B="FAKE-not-a-real-credential-tenant-b"

# Writes one secret per tenant using that tenant's own writer identity, so the
# fixtures themselves prove the positive path before any denial is asserted.
conformance_seed_fixtures() {
  as_token "${TOKEN_A_WRITER}" POST "$(kv_data "${TENANT_A}" "${FIXTURE_SECRET_NAME}")" \
    "$(kv_payload "${FIXTURE_SECRET_KEY}" "${FIXTURE_VALUE_A}")"
  case "${ASSERT_STATUS}" in
    2*) ;;
    *) die "could not seed fixture for ${TENANT_A} (HTTP ${ASSERT_STATUS}) - the writer policy is broken, so no isolation result below would be meaningful" ;;
  esac

  as_token "${TOKEN_B_WRITER}" POST "$(kv_data "${TENANT_B}" "${FIXTURE_SECRET_NAME}")" \
    "$(kv_payload "${FIXTURE_SECRET_KEY}" "${FIXTURE_VALUE_B}")"
  case "${ASSERT_STATUS}" in
    2*) ;;
    *) die "could not seed fixture for ${TENANT_B} (HTTP ${ASSERT_STATUS})" ;;
  esac
}
