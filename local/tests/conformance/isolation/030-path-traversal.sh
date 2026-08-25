#!/usr/bin/env bash
# Parent, sibling, and prefix-anchor denials.
set -euo pipefail

# shellcheck source=../common/setup.sh
. "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)/setup.sh"

conformance_require_env
conformance_login_tenants
conformance_seed_fixtures

suite "Parent paths are denied"

assert_denied "${TOKEN_A_READER}" GET "${KV_MOUNT}/data/${TENANT_PREFIX}" "" \
  "cannot read the tenant prefix itself"

assert_denied "${TOKEN_A_READER}" GET "${KV_MOUNT}/data" "" \
  "cannot read the mount root"

assert_denied "${TOKEN_A_READER}" GET "${KV_MOUNT}/metadata/${TENANT_PREFIX}" "" \
  "cannot read the tenant prefix metadata"

assert_denied "${TOKEN_A_WRITER}" POST "${KV_MOUNT}/data/${TENANT_PREFIX}/shared-secret" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "FAKE-planted-at-parent")" \
  "cannot plant a secret at the shared parent level"

suite "Sibling paths outside the tenant subtree are denied"

assert_denied "${TOKEN_A_READER}" GET "${KV_MOUNT}/data/platform/root-credentials" "" \
  "cannot read a sibling prefix beside the tenant prefix"

assert_denied "${TOKEN_A_WRITER}" POST "${KV_MOUNT}/data/platform/root-credentials" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "FAKE-planted-sibling")" \
  "cannot write to a sibling prefix"

suite "Prefix-anchoring is exact"

# The critical case. A rule ending "tenant-a*" instead of "tenant-a/*" matches
# every tenant whose ID merely starts with the same characters. Nothing else in
# the suite detects this, and it is a plausible typo.
assert_denied "${TOKEN_A_READER}" GET \
  "${KV_MOUNT}/data/${TENANT_PREFIX}/${TENANT_A}-extended/secret" "" \
  "a tenant ID that merely starts with '${TENANT_A}' is not reachable"

assert_denied "${TOKEN_A_READER}" GET \
  "${KV_MOUNT}/data/${TENANT_PREFIX}/${TENANT_A}x/secret" "" \
  "appending a character to the tenant ID does not stay inside the subtree"

suite "Traversal sequences do not escape the subtree"

# Vault normalizes and rejects these, so the expected result is simply "not
# allowed". Asserted anyway: this suite is the contract's evidence that
# traversal was considered, and a future proxy or gateway in front of Vault is
# exactly the component that could reintroduce the problem.
for traversal in \
  "${KV_MOUNT}/data/${TENANT_PREFIX}/${TENANT_A}/../${TENANT_B}/${FIXTURE_SECRET_NAME}" \
  "${KV_MOUNT}/data/${TENANT_PREFIX}/${TENANT_A}/..%2f${TENANT_B}/${FIXTURE_SECRET_NAME}" \
  "${KV_MOUNT}/data/${TENANT_PREFIX}/${TENANT_A}//../${TENANT_B}/${FIXTURE_SECRET_NAME}"
do
  as_token "${TOKEN_A_READER}" GET "${traversal}"
  case "${ASSERT_STATUS}" in
    2*) fail "traversal blocked: ${traversal}" \
             "ACCESS WAS GRANTED (HTTP ${ASSERT_STATUS}) - this reaches another tenant's data" ;;
    *)  pass "traversal blocked (HTTP ${ASSERT_STATUS}): ${traversal##*"${TENANT_A}"/}" ;;
  esac
done

suite "Wildcard characters in a request path grant nothing"

# A tenant asking for a literal wildcard must not receive a wildcard's worth of
# access. Vault treats `*` as a literal path segment, not a glob.
# customers/* is outside this tenant's allow, so that is a hard 403.
# customers/tenant-a/* is inside the allow; a missing literal key is 404.
assert_denied "${TOKEN_A_READER}" GET "${KV_MOUNT}/data/${TENANT_PREFIX}/*" "" \
  "requesting a wildcard path does not match every tenant"

assert_not_granted "${TOKEN_A_READER}" GET "$(kv_data "${TENANT_A}" "")*" "" \
  "requesting a wildcard inside the tenant subtree returns no data"

finish
