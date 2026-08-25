#!/usr/bin/env bash
# After revoke, no Vault-created DB roles remain. Scan is self-tested first.
set -euo pipefail

COMMON_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)"
# shellcheck source=../common/setup.sh
. "${COMMON_DIR}/setup.sh"
# shellcheck source=../common/database.sh
. "${COMMON_DIR}/database.sh"

credentials_require_env
conformance_login_tenants

PROVISIONING_TOKEN="${VAULT_TOKEN}"

# 1. Detector self-test - runs FIRST.
#
# If the scan cannot detect a planted orphan, every result below is
# meaningless, and finding that out after reporting "zero orphans" is the
# failure mode this ordering avoids.
suite "Residue detector self-test"

PLANTED_ROLE="${VAULT_ROLE_PREFIX}selftest-planted-orphan"

db_admin_query "DROP ROLE IF EXISTS \"${PLANTED_ROLE}\";" >/dev/null 2>&1 || true
BASELINE="$(count_vault_db_roles)"

if db_admin_query "CREATE ROLE \"${PLANTED_ROLE}\" NOLOGIN;" >/dev/null 2>&1; then
  pass "planted a deliberate orphan role"
else
  fail "planted a deliberate orphan role" \
       "could not create ${PLANTED_ROLE} as ${DB_ADMIN_USER} - the self-test cannot run, so the scan below is unverified"
  finish
fi

AFTER_PLANT="$(count_vault_db_roles)"

if [ "${AFTER_PLANT}" -gt "${BASELINE}" ]; then
  pass "the scan detects a planted orphan (${BASELINE} -> ${AFTER_PLANT})"
else
  fail "the scan detects a planted orphan" \
       "count did not increase (${BASELINE} -> ${AFTER_PLANT}) - the residue scan is BROKEN and cannot be trusted"
fi

if list_vault_db_roles | grep -q "${PLANTED_ROLE}"; then
  pass "the planted orphan is named in the scan output"
else
  fail "the planted orphan is named in the scan output" \
       "the scan counts roles it cannot list - orphan reports would be unactionable"
fi

db_admin_query "DROP ROLE IF EXISTS \"${PLANTED_ROLE}\";" >/dev/null 2>&1 || true

if db_role_exists "${PLANTED_ROLE}"; then
  fail "planted orphan removed after the self-test" "${PLANTED_ROLE} still exists"
else
  pass "planted orphan removed after the self-test"
fi

# 2. Issue, then revoke everything, then scan.
suite "Issued credentials leave no residue"

ISSUED_USERS=""
ISSUED_RECORDS=""
for role in "tenant-${TENANT_A}-readonly" "tenant-${TENANT_A}-readwrite" "tenant-${TENANT_B}-readonly"; do
  token="${TOKEN_A_WRITER}"
  case "${role}" in *"${TENANT_B}"*) token="${TOKEN_B_WRITER}" ;; esac

  if cred="$(issue_credential "${token}" "${role}")"; then
    IFS=$'\t' read -r u _ lease _ <<< "${cred}"
    ISSUED_USERS="${ISSUED_USERS} ${u}"
    ISSUED_RECORDS="${ISSUED_RECORDS}${token}"$'\t'"${lease}"$'\n'
  fi
done

ISSUED_COUNT="$(printf '%s' "${ISSUED_USERS}" | wc -w | tr -d '[:space:]')"
if [ "${ISSUED_COUNT}" -ge 3 ]; then
  pass "issued ${ISSUED_COUNT} credentials to clean up"
else
  fail "issued credentials to clean up" "only ${ISSUED_COUNT} were issued - the cleanup assertion would be weak"
fi

# Present before revocation. Without this, "no roles found afterwards" could
# simply mean none were ever created.
present=0
for u in ${ISSUED_USERS}; do
  db_role_exists "${u}" && present=$((present + 1))
done
assert_eq "${ISSUED_COUNT}" "${present}" "all issued credentials exist in pg_roles before revocation"

while IFS=$'\t' read -r token lease; do
  [ -n "${lease}" ] || continue
  revoke_lease "${token}" "${lease}" || true
done <<< "${ISSUED_RECORDS}"

# Bounded wait rather than a fixed sleep, for the same reason as in the
# revocation suite: a sleep long enough to always pass hides slow revocation.
cleaned=false
for _ in $(seq 1 15); do
  remaining=0
  for u in ${ISSUED_USERS}; do
    db_role_exists "${u}" && remaining=$((remaining + 1))
  done
  [ "${remaining}" -eq 0 ] && { cleaned=true; break; }
  sleep 1
done

if [ "${cleaned}" = "true" ]; then
  pass "every revoked credential's role was dropped from PostgreSQL"
else
  orphans=""
  for u in ${ISSUED_USERS}; do
    db_role_exists "${u}" && orphans="${orphans} ${u}"
  done
  fail "every revoked credential's role was dropped from PostgreSQL" \
       "orphaned roles remain:${orphans} - these are credentials nobody is tracking"
fi

# 3. Whole-mount scan.
suite "No unaccounted Vault-created roles remain"

FINAL_ROLES="$(list_vault_db_roles | grep -v '^[[:space:]]*$' || true)"
FINAL_COUNT="$(printf '%s' "${FINAL_ROLES}" | grep -c . || true)"

# Roles from a still-live lease are legitimate, so the scan reports what it
# found rather than failing outright - an unexplained count is a finding to
# investigate, not automatically a defect.
if [ "${FINAL_COUNT}" -eq 0 ]; then
  pass "zero ${VAULT_ROLE_PREFIX}* roles remain in pg_roles"
else
  printf '    NOTE  %d %s* role(s) still present:\n' "${FINAL_COUNT}" "${VAULT_ROLE_PREFIX}"
  printf '%s\n' "${FINAL_ROLES}" | sed 's/^/          /'

  VAULT_TOKEN="${PROVISIONING_TOKEN}"
  live_leases=0
  for role in "tenant-${TENANT_A}-readonly" "tenant-${TENANT_A}-readwrite" \
              "tenant-${TENANT_B}-readonly" "tenant-${TENANT_B}-readwrite"; do
    if vault_request GET "sys/leases/lookup/${DATABASE_MOUNT}/creds/${role}?list=true" >/dev/null 2>&1; then
      n="$(vault_body | jq -r '.data.keys | length' 2>/dev/null || echo 0)"
      live_leases=$((live_leases + n))
    fi
  done

  if [ "${live_leases}" -ge "${FINAL_COUNT}" ]; then
    pass "all remaining roles are accounted for by ${live_leases} live lease(s)"
  else
    fail "all remaining roles are accounted for by live leases" \
         "${FINAL_COUNT} role(s) present but only ${live_leases} live lease(s) - the difference is orphaned residue"
  fi
fi

finish
