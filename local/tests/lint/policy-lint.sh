#!/usr/bin/env bash
# Reject ACL policies that silently break isolation.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../scripts/lib/common.sh
. "${SCRIPT_DIR}/../../scripts/lib/common.sh"

platform_defaults

[ $# -ge 1 ] || die "usage: $(basename "$0") <policy-file> [...]"

# KV v2 exposes its API through these path segments. Anything else immediately
# after the mount name is not a real KV v2 path.
KV_V2_SEGMENTS="data metadata delete undelete destroy config subkeys"
VALID_CAPABILITIES="create read update delete list patch sudo deny recover subscribe"

FINDINGS=0

finding() {
  FINDINGS=$((FINDINGS + 1))
  printf '  [%d] %s\n' "${FINDINGS}" "$*" >&2
}

lint_file() {
  local file="$1"
  local policy_name; policy_name="$(basename "${file}" .hcl)"
  local seen_paths=""

  [ -f "${file}" ] || { finding "no such file: ${file}"; return; }

  # Extract "path<TAB>capabilities" pairs. A small awk state machine rather
  # than a real HCL parser: policy files are a narrow, well-known subset of
  # HCL, and adding an HCL toolchain dependency to lint five files is not a
  # trade worth making.
  local pairs
  pairs="$(awk '
    /^[[:space:]]*#/ { next }
    match($0, /path[[:space:]]+"[^"]+"/) {
      p = substr($0, RSTART, RLENGTH)
      sub(/^path[[:space:]]+"/, "", p); sub(/"$/, "", p)
      current = p
      caps = ""
      next
    }
    /capabilities[[:space:]]*=/ {
      line = $0
      sub(/.*\[/, "", line); sub(/\].*/, "", line)
      gsub(/[" ]/, "", line)
      if (current != "") { print current "\t" line; current = "" }
    }
  ' "${file}")"

  if [ -z "${pairs}" ]; then
    finding "${policy_name}: no path rules found - an empty policy grants nothing and is almost certainly a rendering bug"
    return
  fi

  local path caps
  while IFS=$'\t' read -r path caps; do
    [ -n "${path}" ] || continue

    # RULE 1: universal wildcard.
    # Grants or denies the entire Vault API in one line.
    if [ "${path}" = "*" ] || [ "${path}" = "/*" ]; then
      case "${caps}" in
        deny) ;;
        *) finding "${policy_name}: path \"${path}\" grants [${caps}] over the ENTIRE Vault API" ;;
      esac
    fi

    # RULE 2: capability sanity.
    if [ -z "${caps}" ]; then
      finding "${policy_name}: path \"${path}\" has an empty capabilities list"
    fi

    local c found
    for c in ${caps//,/ }; do
      found=0
      for valid in ${VALID_CAPABILITIES}; do
        [ "${c}" = "${valid}" ] && { found=1; break; }
      done
      [ "${found}" -eq 1 ] || finding "${policy_name}: path \"${path}\" has unknown capability '${c}' - Vault ignores unknown capabilities, so this rule grants less than it appears to"
    done

    # deny is absolute and cannot be combined; listing it alongside grants
    # reads as "allow these, deny the rest", which is not what it means.
    case ",${caps}," in
      *,deny,*)
        if [ "${caps}" != "deny" ]; then
          finding "${policy_name}: path \"${path}\" mixes 'deny' with [${caps}] - deny overrides everything else here, so the other capabilities are misleading"
        fi
        ;;
    esac

    # RULE 3: sudo outside the operator policy.
    case ",${caps}," in
      *,sudo,*)
        if [ "${policy_name}" != "operator" ] && [ "${policy_name}" != "admin" ]; then
          finding "${policy_name}: path \"${path}\" grants 'sudo' - root-equivalent on that path, and not appropriate outside a break-glass policy"
        fi
        ;;
    esac

    # RULE 4: KV v2 path-split
    # The highest-probability defect in this platform. A policy written against
    # <mount>/<tenant-prefix>/... matches nothing on KV v2, because values live
    # under <mount>/data/ and history under <mount>/metadata/. It applies
    # cleanly, returns no error, and grants exactly nothing.
    #
    # Checked for grants only. A deny that matches nothing is harmless, and
    # mount-wide catch-all denies like "<mount>/*" are deliberate: they are how
    # the provisioning policy states that it has no business in tenant data at
    # all. Flagging those would train people to ignore this rule, which is the
    # one rule that must never be ignored.
    case "${path}" in
      "${KV_MOUNT}"/*)
        local second; second="$(printf '%s' "${path#"${KV_MOUNT}"/}" | cut -d/ -f1)"
        local ok=0 seg
        [ "${caps}" = "deny" ] && ok=1
        [ "${second}" = "*" ] && ok=1
        for seg in ${KV_V2_SEGMENTS}; do
          [ "${second}" = "${seg}" ] && { ok=1; break; }
        done
        if [ "${ok}" -eq 0 ]; then
          finding "${policy_name}: path \"${path}\" is not a valid KV v2 path.
        KV v2 stores values under ${KV_MOUNT}/data/... and version history under
        ${KV_MOUNT}/metadata/... . This rule matches nothing, applies without
        error, and grants no access. Did you mean
        \"${KV_MOUNT}/data/${path#"${KV_MOUNT}"/}\"?"
        fi
        ;;
    esac

    # RULE 5: cross-tenant wildcard.
    # A grant on <mount>/<family>/<prefix>/* covers EVERY tenant. Only a deny
    # belongs at that level; the tenant-scoped grant must include the tenant
    # segment.
    case "${path}" in
      "${KV_MOUNT}"/*/"${TENANT_PREFIX}"/\*|"${KV_MOUNT}"/*/"${TENANT_PREFIX}"/)
        if [ "${caps}" != "deny" ]; then
          finding "${policy_name}: path \"${path}\" grants [${caps}] across ALL tenants.
        A wildcard directly under '${TENANT_PREFIX}/' crosses every tenant
        boundary. Scope it to one tenant, or make it a deny."
        fi
        ;;
    esac

    # Same problem one level up: a grant on the whole mount.
    case "${path}" in
      "${KV_MOUNT}"/\*|"${KV_MOUNT}"/data/\*|"${KV_MOUNT}"/metadata/\*)
        if [ "${caps}" != "deny" ]; then
          finding "${policy_name}: path \"${path}\" grants [${caps}] over the entire KV mount, crossing every tenant boundary"
        fi
        ;;
    esac

    # RULE 6: cross-tenant dynamic credentials.
    case "${path}" in
      "${DATABASE_MOUNT}"/creds/\*)
        if [ "${caps}" != "deny" ]; then
          finding "${policy_name}: path \"${path}\" grants [${caps}] on every tenant's database credentials"
        fi
        ;;
    esac

    # RULE 7: duplicate path stanzas.
    # Vault keeps one of them. Which one is not something to rely on.
    case " ${seen_paths} " in
      *" ${path} "*) finding "${policy_name}: path \"${path}\" is declared more than once - only one stanza takes effect" ;;
      *) seen_paths="${seen_paths} ${path}" ;;
    esac

  done <<< "${pairs}"
}

for f in "$@"; do
  lint_file "${f}"
done

if [ "${FINDINGS}" -gt 0 ]; then
  printf '\npolicy lint FAILED: %d finding(s)\n\n' "${FINDINGS}" >&2
  exit 1
fi

exit 0
