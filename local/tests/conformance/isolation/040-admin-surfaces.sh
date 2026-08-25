#!/usr/bin/env bash
# Tenants cannot reach sys, auth, policy, or token admin.
set -euo pipefail

# shellcheck source=../common/setup.sh
. "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)/setup.sh"

conformance_require_env
conformance_login_tenants

suite "Token self-management is permitted (control)"

# A tenant must be able to manage its own token lifetime. If these fail, the
# denials below could simply mean the token is dead.
assert_allowed "${TOKEN_A_READER}" GET "auth/token/lookup-self" "" \
  "tenant can look up its own token"

assert_allowed "${TOKEN_A_READER}" POST "auth/token/renew-self" '{}' \
  "tenant can renew its own token"

suite "System administration is denied"

assert_denied "${TOKEN_A_READER}" GET  "sys/mounts" "" "cannot enumerate mounts"
assert_denied "${TOKEN_A_READER}" GET  "sys/auth" ""   "cannot enumerate auth methods"
assert_denied "${TOKEN_A_WRITER}" POST "sys/mounts/rogue" \
  '{"type":"kv","options":{"version":"2"}}' "cannot mount a new secrets engine"
assert_denied "${TOKEN_A_WRITER}" DELETE "sys/mounts/${KV_MOUNT}" "" \
  "cannot unmount the KV engine"

suite "Audit configuration is denied"

# An identity that can disable audit can act unobserved, which would defeat the
# evidence trail every other control depends on.
assert_denied "${TOKEN_A_READER}" GET    "sys/audit" ""  "cannot read audit configuration"
assert_denied "${TOKEN_A_WRITER}" DELETE "sys/audit/file" "" "cannot disable the audit device"

suite "Policy administration is denied"

assert_denied "${TOKEN_A_READER}" GET "sys/policies/acl?list=true" "" \
  "cannot list policies"

assert_denied "${TOKEN_A_READER}" GET \
  "sys/policies/acl/$(tenant_policy_reader "${TENANT_B}")" "" \
  "cannot read another tenant's policy"

# The direct escalation: rewrite your own policy to grant everything.
assert_denied "${TOKEN_A_WRITER}" PUT \
  "sys/policies/acl/$(tenant_policy_reader "${TENANT_A}")" \
  '{"policy":"path \"kv/data/*\" { capabilities = [\"read\", \"list\"] }"}' \
  "cannot rewrite its own policy to widen access"

assert_denied "${TOKEN_A_WRITER}" PUT "sys/policies/acl/escalated" \
  '{"policy":"path \"*\" { capabilities = [\"sudo\", \"read\"] }"}' \
  "cannot create a new privileged policy"

suite "Auth administration is denied"

assert_denied "${TOKEN_A_READER}" GET "auth/${AUTH_MOUNT}/role?list=true" "" \
  "cannot enumerate AppRole roles"

assert_denied "${TOKEN_A_READER}" GET \
  "auth/${AUTH_MOUNT}/role/$(tenant_role_reader "${TENANT_B}")/role-id" "" \
  "cannot read another tenant's role-id"

# The indirect escalation: mint a credential for another tenant's role and log
# in as them. Closing the data path is not enough if the identity path is open.
assert_denied "${TOKEN_A_WRITER}" POST \
  "auth/${AUTH_MOUNT}/role/$(tenant_role_reader "${TENANT_B}")/secret-id" '{}' \
  "cannot issue a secret-id for another tenant's role"

assert_denied "${TOKEN_A_WRITER}" POST \
  "auth/${AUTH_MOUNT}/role/tenant-rogue-writer" \
  '{"token_policies":["operator"]}' \
  "cannot create an AppRole role bound to a privileged policy"

suite "Token administration is denied"

# Creating a token with different policies is the shortest escalation path
# there is, and it leaves a token that outlives the request.
assert_denied "${TOKEN_A_WRITER}" POST "auth/token/create" \
  '{"policies":["operator"]}' \
  "cannot create a token carrying another policy"

assert_denied "${TOKEN_A_WRITER}" POST "auth/token/create-orphan" \
  '{"policies":["provisioning"]}' \
  "cannot create an orphan token carrying the provisioning policy"

assert_denied "${TOKEN_A_READER}" GET "auth/token/accessors?list=true" "" \
  "cannot enumerate token accessors"

suite "Identity secrets engine is denied"

# Entity aliases can attach additional policies to an identity, which is
# policy administration through a different endpoint.
assert_denied "${TOKEN_A_WRITER}" POST "identity/entity" \
  '{"name":"rogue","policies":["operator"]}' \
  "cannot create an identity entity with elevated policies"

suite "Other secrets engines are denied"

# Cubbyhole is per-token. Another token's keys are not addressable, so a
# missing name is 404 rather than ACL 403. Granting 2xx would be the breach.
assert_not_granted "${TOKEN_A_READER}" GET "cubbyhole/other" "" \
  "cannot read another token's cubbyhole"

assert_denied "${TOKEN_A_READER}" GET "${DATABASE_MOUNT}/config/app" "" \
  "cannot read the database engine's connection configuration"

finish
