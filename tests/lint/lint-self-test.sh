#!/usr/bin/env bash
# Linter must reject every BAD-* fixture and accept GOOD-*.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
LINTER="${SCRIPT_DIR}/policy-lint.sh"
FIXTURE_DIR="${SCRIPT_DIR}/fixtures"

PASSED=0
FAILED=0

ok()   { printf '  PASS  %s\n' "$*"; PASSED=$((PASSED + 1)); }
bad()  { printf '  FAIL  %s\n' "$*"; FAILED=$((FAILED + 1)); }

printf '\nPolicy linter self-test\n\n'

# Fixtures named BAD-* must be rejected. If the linter accepts one, the rule it
# tests is gone or broken - which is the case this whole file exists to catch.
for fixture in "${FIXTURE_DIR}"/BAD-*.hcl; do
  name="$(basename "${fixture}")"
  if "${LINTER}" "${fixture}" >/dev/null 2>&1; then
    bad "${name} was ACCEPTED but must be rejected"
  else
    ok "${name} correctly rejected"
  fi
done

# Fixtures named GOOD-* must pass. Without this direction, a linter that
# rejects everything unconditionally would score a perfect result above.
for fixture in "${FIXTURE_DIR}"/GOOD-*.hcl; do
  name="$(basename "${fixture}")"
  if "${LINTER}" "${fixture}" >/dev/null 2>&1; then
    ok "${name} correctly accepted"
  else
    bad "${name} was REJECTED but must pass"
    printf '        linter output:\n'
    "${LINTER}" "${fixture}" 2>&1 | sed 's/^/        /' || true
  fi
done

printf '\n  %d passed, %d failed\n\n' "${PASSED}" "${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
