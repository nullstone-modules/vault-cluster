#!/usr/bin/env bash
# Cross-tenant matrix. Denials must be HTTP 403.
set -euo pipefail

# shellcheck source=../common/setup.sh
. "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)/setup.sh"

conformance_require_env
conformance_login_tenants
conformance_seed_fixtures

A_SECRET="$(kv_data "${TENANT_A}" "${FIXTURE_SECRET_NAME}")"
B_SECRET="$(kv_data "${TENANT_B}" "${FIXTURE_SECRET_NAME}")"
A_META="$(kv_meta "${TENANT_A}" "${FIXTURE_SECRET_NAME}")"
B_META="$(kv_meta "${TENANT_B}" "${FIXTURE_SECRET_NAME}")"

# The allow half of the matrix runs first. Without it, a Vault that denies
# everything - a broken mount, a failed bootstrap, an expired token - would
# score a perfect pass on every deny assertion below.
suite "Same-tenant access is permitted (control)"

assert_allowed "${TOKEN_A_READER}" GET "${A_SECRET}" "" "${TENANT_A} reads ${TENANT_A}"
assert_allowed "${TOKEN_B_READER}" GET "${B_SECRET}" "" "${TENANT_B} reads ${TENANT_B}"

suite "Cross-tenant read is denied"

assert_denied "${TOKEN_A_READER}" GET "${B_SECRET}" "" "${TENANT_A} reader cannot read ${TENANT_B}"
assert_denied "${TOKEN_B_READER}" GET "${A_SECRET}" "" "${TENANT_B} reader cannot read ${TENANT_A}"
assert_denied "${TOKEN_A_WRITER}" GET "${B_SECRET}" "" "${TENANT_A} writer cannot read ${TENANT_B}"
assert_denied "${TOKEN_B_WRITER}" GET "${A_SECRET}" "" "${TENANT_B} writer cannot read ${TENANT_A}"

suite "Cross-tenant write is denied"

# Checked independently of read. A policy that denies read while permitting
# write would let one tenant silently corrupt another's secrets - arguably
# worse than disclosure, and invisible to a read-only test.
assert_denied "${TOKEN_A_WRITER}" POST "${B_SECRET}" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "FAKE-cross-tenant-write")" \
  "${TENANT_A} writer cannot write into ${TENANT_B}"

assert_denied "${TOKEN_B_WRITER}" POST "${A_SECRET}" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "FAKE-cross-tenant-write")" \
  "${TENANT_B} writer cannot write into ${TENANT_A}"

# A path that does not exist yet, so a permissive policy would return 404
# rather than 403. Asserting 403 proves the denial comes from authorization and
# not from absence.
assert_denied "${TOKEN_A_WRITER}" POST "$(kv_data "${TENANT_B}" "newly-planted-secret")" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "FAKE-planted")" \
  "${TENANT_A} cannot create a NEW secret inside ${TENANT_B}"

suite "Cross-tenant delete and destroy are denied"

assert_denied "${TOKEN_A_WRITER}" DELETE "${B_SECRET}" "" \
  "${TENANT_A} cannot soft-delete ${TENANT_B}'s secret"

assert_denied "${TOKEN_A_WRITER}" POST \
  "${KV_MOUNT}/destroy/${TENANT_PREFIX}/${TENANT_B}/${FIXTURE_SECRET_NAME}" \
  '{"versions":[1]}' \
  "${TENANT_A} cannot destroy ${TENANT_B}'s secret versions"

assert_denied "${TOKEN_A_WRITER}" DELETE "${B_META}" "" \
  "${TENANT_A} cannot delete ${TENANT_B}'s metadata"

suite "Cross-tenant metadata access is denied"

# Metadata is a separate KV v2 API surface. A policy covering data/ but not
# metadata/ leaks version counts, timestamps, and key names across tenants
# while every data-path test still passes.
assert_denied "${TOKEN_A_READER}" GET "${B_META}" "" \
  "${TENANT_A} cannot read ${TENANT_B}'s secret metadata"

assert_denied "${TOKEN_B_READER}" GET "${A_META}" "" \
  "${TENANT_B} cannot read ${TENANT_A}'s secret metadata"

suite "Tenant enumeration is denied"

# Listing the shared parent would reveal the full customer list. That is a
# commercially sensitive disclosure even when no secret value is exposed.
assert_denied "${TOKEN_A_READER}" GET "${KV_MOUNT}/metadata/${TENANT_PREFIX}?list=true" "" \
  "${TENANT_A} cannot list the tenant prefix"

assert_denied "${TOKEN_B_READER}" GET "${KV_MOUNT}/metadata/${TENANT_PREFIX}?list=true" "" \
  "${TENANT_B} cannot list the tenant prefix"

assert_denied "${TOKEN_A_READER}" GET "$(kv_meta "${TENANT_B}" "")?list=true" "" \
  "${TENANT_A} cannot list inside ${TENANT_B}'s subtree"

finish
