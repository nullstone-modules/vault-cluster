#!/usr/bin/env bash
# Assertion harness for conformance tests.
#
# Sourced, not executed.
#
# Runtime-independent: every assertion is expressed against VAULT_ADDR over
# HTTP. No container names, no docker, no host paths - so the same suite runs
# against Compose today and a cluster later without modification.
#
# The central design decision here is that a denial must be proven by HTTP 403,
# never by "the request failed". A test that accepts any failure as a denial
# passes against an empty Vault, against an unreachable Vault, and against a
# Vault where the path simply does not exist - none of which say anything about
# authorization. That distinction is the difference between an isolation suite
# and a suite that looks like one.

# shellcheck shell=bash

TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

suite() {
  printf '\n  %s\n' "$1"
}

pass() {
  TESTS_RUN=$((TESTS_RUN + 1)); TESTS_PASSED=$((TESTS_PASSED + 1))
  printf '    PASS  %s\n' "$1"
}

fail() {
  TESTS_RUN=$((TESTS_RUN + 1)); TESTS_FAILED=$((TESTS_FAILED + 1))
  printf '    FAIL  %s\n' "$1"
  [ -n "${2:-}" ] && printf '          %s\n' "$2"
  return 0
}

# --- Request helper ----------------------------------------------------------
# Performs a request as a specific token without disturbing the caller's
# VAULT_TOKEN, and leaves the status in ASSERT_STATUS and the body in
# ASSERT_BODY.
as_token() {
  local token="$1" method="$2" path="$3" data="${4:-}"
  local saved="${VAULT_TOKEN:-}"
  VAULT_TOKEN="${token}"
  vault_request "${method}" "${path}" "${data}" >/dev/null 2>&1 || true
  ASSERT_STATUS="${VAULT_STATUS}"
  ASSERT_BODY="$(vault_body)"
  VAULT_TOKEN="${saved}"
}

# --- Core assertions ---------------------------------------------------------

# assert_allowed TOKEN METHOD PATH BODY DESCRIPTION
assert_allowed() {
  local token="$1" method="$2" path="$3" data="$4" desc="$5"
  as_token "${token}" "${method}" "${path}" "${data}"
  case "${ASSERT_STATUS}" in
    2*) pass "${desc}" ;;
    403) fail "${desc}" "expected success, got 403 permission denied on ${method} ${path}" ;;
    *)   fail "${desc}" "expected 2xx, got ${ASSERT_STATUS} on ${method} ${path}: $(printf '%s' "${ASSERT_BODY}" | head -c 200)" ;;
  esac
}

# assert_denied TOKEN METHOD PATH BODY DESCRIPTION
#
# Requires exactly 403. A 404 is reported as a distinct, louder failure: it
# usually means the test is pointing at a path that does not exist, so the
# "denial" it was about to record would have been meaningless.
assert_denied() {
  local token="$1" method="$2" path="$3" data="$4" desc="$5"
  as_token "${token}" "${method}" "${path}" "${data}"
  case "${ASSERT_STATUS}" in
    403) pass "${desc}" ;;
    2*)  fail "${desc}" "ACCESS WAS GRANTED (HTTP ${ASSERT_STATUS}) on ${method} ${path} - this is an isolation breach" ;;
    404) fail "${desc}" "got 404 not 403 on ${method} ${path} - the path may not exist, so this test proves nothing about authorization" ;;
    *)   fail "${desc}" "expected 403, got ${ASSERT_STATUS} on ${method} ${path}" ;;
  esac
}

assert_status() {
  local expected="$1" actual="$2" desc="$3"
  if [ "${actual}" = "${expected}" ]; then
    pass "${desc}"
  else
    fail "${desc}" "expected HTTP ${expected}, got ${actual}"
  fi
}

assert_eq() {
  local expected="$1" actual="$2" desc="$3"
  if [ "${expected}" = "${actual}" ]; then
    pass "${desc}"
  else
    fail "${desc}" "expected '${expected}', got '${actual}'"
  fi
}

assert_not_empty() {
  local value="$1" desc="$2"
  if [ -n "${value}" ] && [ "${value}" != "null" ]; then
    pass "${desc}"
  else
    fail "${desc}" "value was empty or null"
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" desc="$3"
  case "${haystack}" in
    *"${needle}"*) pass "${desc}" ;;
    *) fail "${desc}" "expected to find '${needle}'" ;;
  esac
}

assert_command_succeeds() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    pass "${desc}"
  else
    fail "${desc}" "command failed: $*"
  fi
}

assert_command_fails() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    fail "${desc}" "command unexpectedly SUCCEEDED: $*"
  else
    pass "${desc}"
  fi
}

# assert_not_granted TOKEN METHOD PATH BODY DESCRIPTION
#
# Passes on 403 or 404. Use only when success would be a breach, but Vault
# has no ACL 403 because the path is inside an allowed subtree and missing
# (404) or because the backend isolates by identity (cubbyhole). Still fails
# on 2xx: that is granted access.
assert_not_granted() {
  local token="$1" method="$2" path="$3" data="$4" desc="$5"
  as_token "${token}" "${method}" "${path}" "${data}"
  case "${ASSERT_STATUS}" in
    403|404) pass "${desc}" ;;
    2*)  fail "${desc}" "ACCESS WAS GRANTED (HTTP ${ASSERT_STATUS}) on ${method} ${path} - this is an isolation breach" ;;
    *)   fail "${desc}" "expected 403 or 404, got ${ASSERT_STATUS} on ${method} ${path}" ;;
  esac
}

finish() {
  printf '\n  %d run, %d passed, %d failed\n\n' \
    "${TESTS_RUN}" "${TESTS_PASSED}" "${TESTS_FAILED}"
  [ "${TESTS_FAILED}" -eq 0 ] || exit 1
  exit 0
}
