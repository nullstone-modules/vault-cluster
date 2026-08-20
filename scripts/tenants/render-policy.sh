#!/usr/bin/env bash
# render-policy.sh <template> [tenant-id] [policy-name]
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/common.sh
. "${SCRIPT_DIR}/../lib/common.sh"
# shellcheck source=../validation/validate-tenant-id.sh
. "${SCRIPT_DIR}/../validation/validate-tenant-id.sh"

platform_defaults

[ $# -ge 1 ] || die "usage: $(basename "$0") <template-name> [tenant-id] [policy-name]"

TEMPLATE_NAME="$1"
TENANT_ID="${2:-}"
# The rendered file is named after the policy it will become, so the file on
# disk and the policy in Vault cannot drift apart. Callers that know the policy
# name pass it explicitly rather than having this script re-derive it.
POLICY_NAME="${3:-${TEMPLATE_NAME}}"
TEMPLATE_FILE="${POLICY_TEMPLATE_DIR}/${TEMPLATE_NAME}.hcl.tpl"

[ -f "${TEMPLATE_FILE}" ] || die "no such template: ${TEMPLATE_FILE}"

# Re-validated here even though create-tenant.sh already did. This script is
# the last point before an ID becomes an ACL path, and a validation that only
# runs on one code path is a validation that will eventually be bypassed.
if [ -n "${TENANT_ID}" ]; then
  validate_tenant_id "${TENANT_ID}" || die "refusing to render a policy for an invalid tenant ID"
fi

OUTPUT_FILE="${RENDERED_POLICY_DIR}/${POLICY_NAME}.hcl"

mkdir -p "${RENDERED_POLICY_DIR}"

# sed with a fixed, closed set of placeholders rather than envsubst or eval.
# envsubst is not installed by default on macOS, and eval on a policy template
# would execute whatever a template contains.
#
# The substituted values are safe by construction: TENANT_ID has passed
# validation, and the mount names come from the contract's own defaults.
sed \
  -e "s|@@TENANT_ID@@|${TENANT_ID}|g" \
  -e "s|@@KV_MOUNT@@|${KV_MOUNT}|g" \
  -e "s|@@TENANT_PREFIX@@|${TENANT_PREFIX}|g" \
  -e "s|@@DATABASE_MOUNT@@|${DATABASE_MOUNT}|g" \
  -e "s|@@AUTH_MOUNT@@|${AUTH_MOUNT}|g" \
  "${TEMPLATE_FILE}" > "${OUTPUT_FILE}.tmp"

# A leftover placeholder means an unset variable. Applying such a policy would
# create a rule for a literal path containing "@@", which matches nothing and
# therefore grants nothing - a silent no-op. Fail instead.
if grep -q '@@[A-Z_]*@@' "${OUTPUT_FILE}.tmp"; then
  local_leftover="$(grep -o '@@[A-Z_]*@@' "${OUTPUT_FILE}.tmp" | sort -u | tr '\n' ' ')"
  rm -f "${OUTPUT_FILE}.tmp"
  die "unsubstituted placeholders remain: ${local_leftover}- refusing to write a partial policy"
fi

mv "${OUTPUT_FILE}.tmp" "${OUTPUT_FILE}"
chmod 600 "${OUTPUT_FILE}"

info "rendered ${TEMPLATE_NAME}$([ -n "${TENANT_ID}" ] && printf ' for %s' "${TENANT_ID}") -> ${OUTPUT_FILE}"
printf '%s\n' "${OUTPUT_FILE}"
