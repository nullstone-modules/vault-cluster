#!/usr/bin/env bash
# Provisioning cannot read tenant secrets. Wrong identity is denied.
set -euo pipefail

# shellcheck source=../common/setup.sh
. "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)/setup.sh"

conformance_require_env
conformance_login_tenants
conformance_seed_fixtures

PROVISIONING_TOKEN="${VAULT_TOKEN}"

A_SECRET="$(kv_data "${TENANT_A}" "${FIXTURE_SECRET_NAME}")"
A_META="$(kv_meta "${TENANT_A}" "${FIXTURE_SECRET_NAME}")"

suite "Provisioning identity can provision (control)"

# Establishes that the token is alive and privileged. Without this, every
# denial below could be explained by an expired or invalid token.
assert_allowed "${PROVISIONING_TOKEN}" GET \
  "auth/${AUTH_MOUNT}/role/$(tenant_role_reader "${TENANT_A}")/role-id" "" \
  "provisioning can read a tenant role-id"

assert_allowed "${PROVISIONING_TOKEN}" GET \
  "sys/policies/acl/$(tenant_policy_reader "${TENANT_A}")" "" \
  "provisioning can read a tenant policy"

suite "Provisioning identity cannot read tenant secrets"

# Creating a tenant and reading a tenant are different privileges; automation
# needs only the first. If these pass as ALLOW, a compromised onboarding
# pipeline reads every tenant's secrets at once.
assert_denied "${PROVISIONING_TOKEN}" GET  "${A_SECRET}" "" \
  "provisioning cannot read a tenant secret"

assert_denied "${PROVISIONING_TOKEN}" GET  "${A_META}" "" \
  "provisioning cannot read tenant secret metadata"

assert_denied "${PROVISIONING_TOKEN}" POST "${A_SECRET}" \
  "$(kv_payload "${FIXTURE_SECRET_KEY}" "FAKE-written-by-provisioning")" \
  "provisioning cannot write into a tenant subtree"

assert_denied "${PROVISIONING_TOKEN}" DELETE "${A_SECRET}" "" \
  "provisioning cannot delete a tenant secret"

assert_denied "${PROVISIONING_TOKEN}" GET \
  "${KV_MOUNT}/metadata/${TENANT_PREFIX}?list=true" "" \
  "provisioning cannot enumerate tenants through the KV mount"

suite "Provisioning identity cannot escalate"

# Scoped to tenant-* precisely so it cannot rewrite the policy that constrains
# it. Without this boundary, every deny above is one API call away from being
# removed by the identity they constrain.
assert_denied "${PROVISIONING_TOKEN}" PUT "sys/policies/acl/provisioning" \
  '{"policy":"path \"kv/data/*\" { capabilities = [\"read\"] }"}' \
  "provisioning cannot rewrite its own policy"

assert_denied "${PROVISIONING_TOKEN}" PUT "sys/policies/acl/operator" \
  '{"policy":"path \"*\" { capabilities = [\"sudo\"] }"}' \
  "provisioning cannot rewrite the operator policy"

assert_denied "${PROVISIONING_TOKEN}" DELETE "sys/audit/file" "" \
  "provisioning cannot disable audit logging"

assert_denied "${PROVISIONING_TOKEN}" POST "auth/token/create" \
  '{"policies":["operator"]}' \
  "provisioning cannot mint a token with another policy"

suite "A token with no relevant policy is refused"

# Vault's default-deny, asserted rather than assumed. The 'default' policy is
# attached to every token, so what it permits is a platform-wide floor.
DEFAULT_TOKEN="$(
  vault_request POST "auth/token/create" \
    '{"policies":["default"],"ttl":"5m","no_default_policy":false}' >/dev/null 2>&1
  vault_body | jq -r '.auth.client_token // empty'
)"

if [ -n "${DEFAULT_TOKEN}" ]; then
  assert_denied "${DEFAULT_TOKEN}" GET "${A_SECRET}" "" \
    "a token holding only the default policy cannot read tenant secrets"
  assert_denied "${DEFAULT_TOKEN}" GET "${KV_MOUNT}/metadata/${TENANT_PREFIX}?list=true" "" \
    "a token holding only the default policy cannot enumerate tenants"
else
  # Expected when the suite runs as provisioning, which cannot mint tokens.
  # Reported rather than skipped silently: an unrun assertion that prints
  # nothing is indistinguishable from one that passed.
  printf '    SKIP  default-policy token checks (this identity cannot create tokens)\n'
fi

suite "The wrong tenant's policy grants nothing"

# Distinct from the cross-tenant matrix: there the token was correctly bound
# and asked for the wrong data. Here the binding itself is the subject - a
# tenant holding tenant B's identity gets tenant B's access and no more, which
# confirms access follows the policy rather than anything ambient.
assert_denied "${TOKEN_B_READER}" GET "${A_SECRET}" "" \
  "${TENANT_B}'s identity cannot read ${TENANT_A}'s secret"

assert_allowed "${TOKEN_B_READER}" GET "$(kv_data "${TENANT_B}" "${FIXTURE_SECRET_NAME}")" "" \
  "${TENANT_B}'s identity reads exactly its own secret"

finish
