#!/usr/bin/env bash
# validate-tenant-id.sh <id>  — reject IDs that would break ACL paths.
set -euo pipefail

# shellcheck source=../lib/common.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/../lib/common.sh"

TENANT_ID_PATTERN='^[a-z0-9]([a-z0-9-]{1,30}[a-z0-9])$'

TENANT_ID_RESERVED="sys auth identity cubbyhole root default admin data metadata delete undelete destroy config subkeys tenant customers"

validate_tenant_id() {
  local id="${1-}"

  [ -n "${id}" ] || { warn "tenant ID is empty"; return 1; }

  # Checked before anything else: a leading or trailing space is invisible in
  # a terminal and would otherwise be reported as a pattern mismatch.
  case "${id}" in
    *[[:space:]]*) warn "tenant ID contains whitespace: '${id}'"; return 1 ;;
  esac

  case "${id}" in
    */*)   warn "tenant ID contains a path separator '/': '${id}' - this would escape the tenant subtree"; return 1 ;;
    *..*)  warn "tenant ID contains '..': '${id}' - path traversal is not permitted"; return 1 ;;
    *'*'*) warn "tenant ID contains the ACL wildcard '*': '${id}' - this would widen every generated policy"; return 1 ;;
    *'+'*) warn "tenant ID contains '+': '${id}' - this is a Vault path segment wildcard"; return 1 ;;
    *'{'*|*'}'*) warn "tenant ID contains a brace: '${id}' - this collides with Vault policy templating"; return 1 ;;
    *'\'*) warn "tenant ID contains a backslash: '${id}'"; return 1 ;;
    *'"'*|*"'"*) warn "tenant ID contains a quote: '${id}'"; return 1 ;;
    *'$'*|*'`'*) warn "tenant ID contains a shell metacharacter: '${id}'"; return 1 ;;
    *%*)   warn "tenant ID contains '%': '${id}' - percent-encoding is not permitted"; return 1 ;;
  esac

  # Non-ASCII: homoglyphs make two visually identical tenant IDs that are
  # different paths, which is a review-time trap rather than a runtime error.
  case "${id}" in
    *[![:ascii:]]*) warn "tenant ID contains non-ASCII characters: '${id}'"; return 1 ;;
  esac

  if ! printf '%s' "${id}" | grep -Eq "${TENANT_ID_PATTERN}"; then
    warn "tenant ID '${id}' does not match ${TENANT_ID_PATTERN}"
    warn "  required: lowercase letters, digits, and hyphens; 3-32 characters;"
    warn "            must start and end with a letter or digit"
    return 1
  fi

  local reserved
  for reserved in ${TENANT_ID_RESERVED}; do
    if [ "${id}" = "${reserved}" ]; then
      warn "tenant ID '${id}' is reserved (collides with a Vault path segment)"
      return 1
    fi
  done

  return 0
}

# Only run the CLI behaviour when executed directly, so create-tenant.sh can
# source this file and call the function.
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  [ $# -eq 1 ] || die "usage: $(basename "$0") <tenant-id>"
  validate_tenant_id "$1" || exit 1
  printf '%s\n' "$1"
fi
