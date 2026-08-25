#!/usr/bin/env bash
# Configure an initialized Vault: audit, KV, AppRole, policies, optional DB.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
. "${SCRIPT_DIR}/lib/common.sh"
# shellcheck source=lib/vault.sh
. "${SCRIPT_DIR}/lib/vault.sh"

require_cmd curl jq sed
platform_defaults
vault_require_env

info "configuring platform at ${VAULT_ADDR}"
info "  kv mount           ${KV_MOUNT}/"
info "  tenant prefix      ${TENANT_PREFIX}/"
info "  auth mount         ${AUTH_MOUNT}/"
info "  dynamic creds      ${ENABLE_DYNAMIC_CREDENTIALS}"

# 1. Audit first.
# Order matters: everything after this point is recorded. Enabling audit last
# would leave the creation of mounts, policies, and identities - the most
# security-relevant operations the platform ever performs - unrecorded.
"${CONFIG_DIR}/audit/file-device.sh"

# 2. Isolation: KV v2 and AppRole.
"${CONFIG_DIR}/secret-engines/kv.sh"
"${CONFIG_DIR}/auth/approle.sh"

# 3. Platform policies.
# Rendered from templates because mount names are configurable; a policy naming
# the wrong mount matches nothing and silently grants nothing.
apply_platform_policy() {
  local name="$1" rendered
  rendered="$("${SCRIPT_DIR}/tenants/render-policy.sh" "${name}" | tail -n 1)"
  "${TESTS_DIR}/lint/policy-lint.sh" "${rendered}" \
    || die "policy '${name}' failed lint - refusing to apply it"
  vault_write_policy "${name}" "${rendered}"
  info "applied policy '${name}'"
}

apply_platform_policy provisioning
apply_platform_policy operator

# 4. Dynamic database credentials. No-op when disabled.
"${CONFIG_DIR}/secret-engines/database.sh"

info "platform configuration complete"
