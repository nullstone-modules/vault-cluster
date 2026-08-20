#!/usr/bin/env bash
# Audit records allow/deny/login. Raw secret values must not appear.
set -euo pipefail

# shellcheck source=../common/setup.sh
. "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../common" && pwd)/setup.sh"

conformance_require_env
conformance_login_tenants
conformance_seed_fixtures

suite "Audit device is enabled"

# Checked with the caller's token; provisioning is denied sys/audit by design,
# so a 403 here confirms the device question is answerable only by an operator.
as_token "${VAULT_TOKEN}" GET "sys/audit"
case "${ASSERT_STATUS}" in
  2*)
    if printf '%s' "${ASSERT_BODY}" | jq -e '(.data // .) | to_entries | length > 0' >/dev/null 2>&1; then
      pass "at least one audit device is enabled"
    else
      fail "at least one audit device is enabled" \
           "sys/audit returned no devices - Vault is running with no audit trail"
    fi
    ;;
  403) printf '    SKIP  sys/audit not readable by this identity (expected for provisioning)\n' ;;
  *)   fail "at least one audit device is enabled" "unexpected HTTP ${ASSERT_STATUS}" ;;
esac

if [ -z "${AUDIT_READ_CMD:-}" ]; then
  # Deliberately no example command here. Naming one would hardcode a specific
  # runtime into a suite whose entire purpose is to be runtime-independent -
  # the target's README is where that belongs.
  printf '\n    SKIP  audit content assertions: AUDIT_READ_CMD is not set.\n'
  printf '          Set it to a command that prints the audit log for this\n'
  printf '          environment; see local/README.md for the local value.\n'
  finish
fi

# Generate one request of each kind, then inspect the log.
A_SECRET="$(kv_data "${TENANT_A}" "${FIXTURE_SECRET_NAME}")"
B_SECRET="$(kv_data "${TENANT_B}" "${FIXTURE_SECRET_NAME}")"

as_token "${TOKEN_A_READER}" GET "${A_SECRET}"   # expected allow
as_token "${TOKEN_A_READER}" GET "${B_SECRET}"   # expected deny

# The file audit device writes synchronously before the response is returned,
# so no sleep is needed. A sleep here would mask a genuinely broken device by
# making the test pass whenever the log eventually caught up.
#
# Grep a temp file. Do not load the log into a bash string and printf it
# through a pipe: grep -q closes on the first match, printf gets SIGPIPE, and
# with pipefail the `if` looks like "not found" even when the needle is present.
AUDIT_LOG_FILE="$(mktemp "${TMPDIR:-/tmp}/vault-audit.XXXXXX")"
chmod 600 "${AUDIT_LOG_FILE}"
if ! eval "${AUDIT_READ_CMD}" >"${AUDIT_LOG_FILE}" 2>/dev/null; then
  rm -f "${AUDIT_LOG_FILE}"
  fail "audit log is readable" "AUDIT_READ_CMD failed: ${AUDIT_READ_CMD}"
  finish
fi

suite "Audit log content"

if [ ! -s "${AUDIT_LOG_FILE}" ]; then
  rm -f "${AUDIT_LOG_FILE}"
  fail "audit log is readable" "AUDIT_READ_CMD produced no output: ${AUDIT_READ_CMD}"
  finish
fi
pass "audit log is readable"

if grep -F -q '"type":"response"' "${AUDIT_LOG_FILE}"; then
  pass "audit log contains response entries"
else
  fail "audit log contains response entries" "expected to find '\"type\":\"response\"'"
fi

# Vault writes a request and a response entry for every operation, including
# denied ones. A device that logged only successes would omit precisely the
# events an investigation needs.
if grep -F -q "${TENANT_PREFIX}/${TENANT_A}" "${AUDIT_LOG_FILE}"; then
  pass "the allowed read was recorded"
else
  fail "the allowed read was recorded" "no entry references ${TENANT_PREFIX}/${TENANT_A}"
fi

if grep -F -q 'permission denied' "${AUDIT_LOG_FILE}"; then
  pass "the denied cross-tenant request was recorded"
else
  fail "the denied cross-tenant request was recorded" \
       "no 'permission denied' entry found - denials are the events that matter most"
fi

if grep -F -q "auth/${AUTH_MOUNT}/login" "${AUDIT_LOG_FILE}"; then
  pass "AppRole authentication was recorded"
else
  fail "AppRole authentication was recorded" "no login entry found"
fi

suite "Audit log does not contain raw secret values"

# The reason log_raw stays false. If secret values were written here, the audit
# log would become the most valuable secret store in the system - and one that
# is deliberately shipped to log aggregators.
for fixture_value in "${FIXTURE_VALUE_A}" "${FIXTURE_VALUE_B}"; do
  if grep -F -q "${fixture_value}" "${AUDIT_LOG_FILE}"; then
    fail "raw secret value is absent from the audit log" \
         "PLAINTEXT SECRET FOUND IN AUDIT LOG: '${fixture_value}' - check that log_raw is false"
  else
    pass "raw secret value '${fixture_value:0:12}...' is absent from the audit log"
  fi
done

# Positive control for the check above: HMAC output must actually be present,
# otherwise "no plaintext found" could simply mean nothing was logged at all.
if grep -F -q 'hmac-sha256:' "${AUDIT_LOG_FILE}"; then
  pass "values are HMAC-ed rather than omitted"
else
  fail "values are HMAC-ed rather than omitted" \
       "no hmac-sha256 markers found - the absence of plaintext above may be meaningless"
fi

rm -f "${AUDIT_LOG_FILE}"
finish
