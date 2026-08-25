#!/usr/bin/env bash
# run-conformance.sh --layer isolation|credentials|all
# Do not use set -e: one suite failure must not skip the rest.
set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../scripts/lib/common.sh
. "${SCRIPT_DIR}/../scripts/lib/common.sh"

platform_defaults
require_cmd curl jq

LAYER="isolation"
while [ $# -gt 0 ]; do
  case "$1" in
    --layer) shift; LAYER="${1:-}" ;;
    --layer=*) LAYER="${1#*=}" ;;
    -h|--help) sed -n '2,3p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
  shift
done

case "${LAYER}" in
  isolation|credentials|all) ;;
  *) die "unknown layer '${LAYER}' (expected: isolation, credentials, or all)" ;;
esac

[ -n "${VAULT_ADDR:-}" ]  || die "VAULT_ADDR is not set"
[ -n "${VAULT_TOKEN:-}" ] || die "VAULT_TOKEN is not set"

if [ "${LAYER}" = "credentials" ] && ! credentials_enabled; then
  die "the credentials suite was requested but ENABLE_DYNAMIC_CREDENTIALS is not true.
     Refusing to report success for a suite that would not run.
     Start the environment with --with-credentials, or run --layer isolation."
fi

FAILED_SUITES=""
PASSED_SUITES=""

run_suite_dir() {
  local dir="$1" label="$2"
  [ -d "${dir}" ] || { warn "no such suite directory: ${dir}"; return 0; }

  printf '\n===============================================================\n'
  printf '  %s conformance\n' "${label}"
  printf '===============================================================\n'

  local file name
  for file in "${dir}"/*.sh; do
    [ -f "${file}" ] || continue
    name="$(basename "${file}" .sh)"
    printf '\n--- %s ---\n' "${name}"

    if bash "${file}"; then
      PASSED_SUITES="${PASSED_SUITES} ${label}/${name}"
    else
      FAILED_SUITES="${FAILED_SUITES} ${label}/${name}"
    fi
  done
}

printf '\nConformance run\n'
printf '  target      %s\n' "${VAULT_ADDR}"
printf '  layer       %s\n' "${LAYER}"
printf '  tenants     %s, %s\n' "${TENANT_A:-tenant-a}" "${TENANT_B:-tenant-b}"
printf '  kv mount    %s/\n' "${KV_MOUNT}"
[ -n "${AUDIT_READ_CMD:-}" ] || printf '  audit       AUDIT_READ_CMD unset - audit content assertions will be skipped\n'

# The linter needs no Vault and is cheap, so it runs first: a broken policy
# template makes every downstream result unreliable.
printf '\n--- policy linter self-test ---\n'
if "${TESTS_DIR}/lint/lint-self-test.sh"; then
  PASSED_SUITES="${PASSED_SUITES} lint/self-test"
else
  FAILED_SUITES="${FAILED_SUITES} lint/self-test"
fi

case "${LAYER}" in
  isolation)
    run_suite_dir "${TESTS_DIR}/conformance/isolation" "isolation"
    ;;
  credentials)
    run_suite_dir "${TESTS_DIR}/conformance/credentials" "credentials"
    ;;
  all)
    run_suite_dir "${TESTS_DIR}/conformance/isolation" "isolation"
    if credentials_enabled; then
      run_suite_dir "${TESTS_DIR}/conformance/credentials" "credentials"
    else
      printf '\n  credentials suite not enabled - skipping (ENABLE_DYNAMIC_CREDENTIALS=false)\n'
    fi
    ;;
esac

printf '\n===============================================================\n'
printf '  Result\n'
printf '===============================================================\n\n'

for s in ${PASSED_SUITES}; do printf '  PASS  %s\n' "${s}"; done
for s in ${FAILED_SUITES}; do printf '  FAIL  %s\n' "${s}"; done

if [ -n "${FAILED_SUITES}" ]; then
  printf '\nCONFORMANCE FAILED\n'
  printf 'A failure in the isolation layer is a cross-tenant breach, not a flaky test.\n\n'
  exit 1
fi

printf '\nCONFORMANCE PASSED\n\n'
