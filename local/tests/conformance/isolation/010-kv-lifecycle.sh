#!/usr/bin/env bash
# KV v2 lifecycle. Positive path first, then reader cannot mutate.
set -euo pipefail

# shellcheck source=../common/setup.sh
. "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)/setup.sh"

conformance_require_env
conformance_login_tenants

suite "KV v2 lifecycle (positive path)"

A_SECRET="$(kv_data "${TENANT_A}" "${FIXTURE_SECRET_NAME}")"
A_META="$(kv_meta "${TENANT_A}" "${FIXTURE_SECRET_NAME}")"

# KV v2 metadata delete removes every version of this key. Without it, a
# leftover from a previous conformance run makes "exactly two versions" fail
# even though versioning still works. Reset is scoped to this suite's fixture.
as_token "${TOKEN_A_WRITER}" DELETE "${A_META}"
case "${ASSERT_STATUS}" in
  2*|404) ;;
  *) die "could not reset lifecycle fixture for ${TENANT_A} (HTTP ${ASSERT_STATUS})" ;;
esac

assert_allowed "${TOKEN_A_WRITER}" POST "${A_SECRET}" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "${FIXTURE_VALUE_A}")" \
  "writer creates a secret in its own subtree"

assert_allowed "${TOKEN_A_WRITER}" GET "${A_SECRET}" "" \
  "writer reads back its own secret"

# Confirms the value survived the KV v2 data envelope intact. A write that
# nests the payload one level too deep succeeds and reads back as valid JSON,
# so only checking the value catches it.
as_token "${TOKEN_A_WRITER}" GET "${A_SECRET}"
assert_eq "${FIXTURE_VALUE_A}" \
  "$(printf '%s' "${ASSERT_BODY}" | jq -r --arg k "${FIXTURE_SECRET_KEY}" '.data.data[$k] // empty')" \
  "stored value round-trips unchanged"

assert_allowed "${TOKEN_A_WRITER}" POST "${A_SECRET}" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "FAKE-rotated-value-tenant-a")" \
  "writer rotates the secret to a new version"

as_token "${TOKEN_A_WRITER}" GET "${A_META}"
assert_eq "2" \
  "$(printf '%s' "${ASSERT_BODY}" | jq -r '.data.versions | length')" \
  "version history records both versions"

assert_allowed "${TOKEN_A_WRITER}" GET "${A_SECRET}?version=1" "" \
  "an earlier version is retrievable for rollback"

assert_allowed "${TOKEN_A_READER}" GET "${A_SECRET}" "" \
  "reader reads its own tenant's secret"

suite "Reader cannot mutate"

# Each of these is a distinct capability. Granting one by accident while
# intending another is the common policy slip, so they are asserted separately
# rather than as a single "reader cannot write".
assert_denied "${TOKEN_A_READER}" POST "${A_SECRET}" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "FAKE-should-never-be-written")" \
  "reader cannot create or update a secret"

assert_denied "${TOKEN_A_READER}" DELETE "${A_SECRET}" "" \
  "reader cannot soft-delete the latest version"

assert_denied "${TOKEN_A_READER}" POST \
  "${KV_MOUNT}/destroy/${TENANT_PREFIX}/${TENANT_A}/${FIXTURE_SECRET_NAME}" \
  '{"versions":[1]}' \
  "reader cannot permanently destroy a version"

assert_denied "${TOKEN_A_READER}" DELETE "${A_META}" "" \
  "reader cannot delete secret metadata"

suite "Writer lifecycle completes"

assert_allowed "${TOKEN_A_WRITER}" DELETE "${A_SECRET}" "" \
  "writer soft-deletes the latest version"

assert_allowed "${TOKEN_A_WRITER}" POST \
  "${KV_MOUNT}/undelete/${TENANT_PREFIX}/${TENANT_A}/${FIXTURE_SECRET_NAME}" \
  '{"versions":[2]}' \
  "writer restores the soft-deleted version"

finish
