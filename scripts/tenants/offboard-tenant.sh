#!/usr/bin/env bash
# offboard-tenant.sh <tenant-id> --yes [--purge-secrets]
# Revokes access. Secrets stay unless --purge-secrets (needs break-glass).
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/common.sh
. "${SCRIPT_DIR}/../lib/common.sh"
# shellcheck source=../lib/vault.sh
. "${SCRIPT_DIR}/../lib/vault.sh"
# shellcheck source=../validation/validate-tenant-id.sh
. "${SCRIPT_DIR}/../validation/validate-tenant-id.sh"

require_cmd curl jq
platform_defaults

CONFIRMED=false
PURGE_SECRETS=false
TENANT_ID=""

while [ $# -gt 0 ]; do
  case "$1" in
    --yes) CONFIRMED=true ;;
    --purge-secrets) PURGE_SECRETS=true ;;
    -h|--help) sed -n '2,3p' "${BASH_SOURCE[0]}"; exit 0 ;;
    -*) die "unknown option: $1 (try --help)" ;;
    *)  [ -z "${TENANT_ID}" ] || die "only one tenant ID may be given"; TENANT_ID="$1" ;;
  esac
  shift
done

[ -n "${TENANT_ID}" ] || die "usage: $(basename "$0") <tenant-id> --yes [--purge-secrets]"
validate_tenant_id "${TENANT_ID}" || die "refusing to act on an invalid tenant ID"

if [ "${CONFIRMED}" != "true" ]; then
  cat >&2 <<EOF
Refusing to run without explicit confirmation.

This revokes all access for tenant '${TENANT_ID}'. Its tokens stop working
immediately and cannot be restored without re-onboarding.
$([ "${PURGE_SECRETS}" = "true" ] && printf '\n--purge-secrets was given: its secrets will ALSO be destroyed, irreversibly.\n')
Re-run with --yes to proceed.
EOF
  exit 1
fi

vault_require_env

READER_POLICY="$(tenant_policy_reader "${TENANT_ID}")"
WRITER_POLICY="$(tenant_policy_writer "${TENANT_ID}")"
DB_POLICY="$(tenant_policy_database "${TENANT_ID}")"
READER_ROLE="$(tenant_role_reader "${TENANT_ID}")"
WRITER_ROLE="$(tenant_role_writer "${TENANT_ID}")"

info "offboarding tenant '${TENANT_ID}'"

# 1. Auth roles first.
# Order matters. Removing the roles stops new logins immediately; removing
# policies first would leave a window in which a login succeeds and receives a
# token carrying a policy that no longer exists - which fails closed, but
# produces a confusing authorization error rather than a clean "no such role".
for role in "${READER_ROLE}" "${WRITER_ROLE}"; do
  if vault_request DELETE "auth/${AUTH_MOUNT}/role/${role}"; then
    info "  deleted auth role ${role}"
  else
    warn "  auth role ${role} not removed (HTTP ${VAULT_STATUS}) - may not exist"
  fi
done

# 2. Database roles and their live leases - only when dynamic credentials are
# enabled.
# Deleting a role does NOT revoke credentials already issued from it. Without
# the prefix revoke below, a tenant offboarded at 09:00 keeps working database
# credentials until their TTL expires, which is precisely the gap that makes
# offboarding look complete when it is not.
if credentials_enabled; then
  for suffix in readonly readwrite; do
    db_role="tenant-${TENANT_ID}-${suffix}"

    if vault_request PUT "sys/leases/revoke-prefix/${DATABASE_MOUNT}/creds/${db_role}" '{}'; then
      info "  revoked live leases for ${db_role}"
    else
      warn "  could not revoke leases for ${db_role} (HTTP ${VAULT_STATUS})"
    fi

    if vault_request DELETE "${DATABASE_MOUNT}/roles/${db_role}"; then
      info "  deleted database role ${db_role}"
    fi
  done
fi

# 3. Policies.
for policy in "${READER_POLICY}" "${WRITER_POLICY}" "${DB_POLICY}"; do
  if vault_request DELETE "sys/policies/acl/${policy}"; then
    info "  deleted policy ${policy}"
  fi
done

# Purge secrets only with --purge-secrets (provisioning cannot read KV).
if [ "${PURGE_SECRETS}" = "true" ]; then
  meta_path="$(tenant_kv_meta_path "${TENANT_ID}")"
  info "purging secrets under ${meta_path}/"

  if ! vault_request GET "${meta_path}?list=true"; then
    if [ "${VAULT_STATUS}" = "403" ]; then
      die "permission denied listing ${meta_path} (provisioning cannot purge tenant data)"
    fi
    warn "nothing to purge under ${meta_path} (HTTP ${VAULT_STATUS})"
  else
    for key in $(vault_body | jq -r '.data.keys[]? // empty'); do
      # Deleting metadata destroys every version of the secret permanently.
      if vault_request DELETE "${meta_path}/${key%/}"; then
        info "  destroyed ${key%/}"
      else
        warn "  failed to destroy ${key%/} (HTTP ${VAULT_STATUS})"
      fi
    done
  fi
else
  info "secrets under $(tenant_kv_data_path "${TENANT_ID}")/ were NOT deleted"
  info "  re-run with --purge-secrets once retention allows"
fi

rm -f "${RENDERED_POLICY_DIR}/${READER_POLICY}.hcl" \
      "${RENDERED_POLICY_DIR}/${WRITER_POLICY}.hcl" \
      "${RENDERED_POLICY_DIR}/${DB_POLICY}.hcl"

info "tenant '${TENANT_ID}' offboarded"
