#!/usr/bin/env bash
# ./snapshot.sh take|list|verify <file>|restore <file> --yes
set -euo pipefail

# shellcheck source=lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

require_cmd curl jq shasum
load_env

BACKUP_DIR="${BOOTSTRAP_DIR}/backups"

operator_token() {
  read_bootstrap_token "operator.token"
}

cmd_take() {
  ensure_bootstrap_dir
  mkdir -p "${BACKUP_DIR}"; chmod 700 "${BACKUP_DIR}"
  vault_is_sealed && die "Vault is sealed - unseal it before taking a snapshot"
  local stamp file
  stamp="$(date -u '+%Y%m%dT%H%M%SZ')"
  file="${BACKUP_DIR}/vault-${stamp}.snap"
  info "taking Raft snapshot"
  (
    umask 077
    curl -sf --max-time 120 \
      --header "X-Vault-Token: $(operator_token)" \
      "${VAULT_ADDR}/v1/sys/storage/raft/snapshot" -o "${file}"
  ) || die "snapshot failed"
  [ -s "${file}" ] || { rm -f "${file}"; die "snapshot file is empty - refusing to keep it"; }
  shasum -a 256 "${file}" | awk '{print $1}' > "${file}.sha256"
  chmod 600 "${file}.sha256"
  info "snapshot written: ${file}"
  info "  size     $(wc -c < "${file}" | tr -d ' ') bytes"
  info "  sha256   $(cat "${file}.sha256")"
  warn "this file contains EVERY secret in the cluster - treat it as one"
}

cmd_list() {
  [ -d "${BACKUP_DIR}" ] || { info "no snapshots under ${BACKUP_DIR}"; return 0; }
  printf '\nSnapshots in %s:\n\n' "${BACKUP_DIR}"
  ls -1t "${BACKUP_DIR}"/*.snap 2>/dev/null | while IFS= read -r f; do
    printf '  %-44s %10s bytes\n' "$(basename "${f}")" "$(wc -c < "${f}" | tr -d ' ')"
  done
  printf '\n'
}

cmd_verify() {
  local file="${1:?usage: snapshot.sh verify <file>}"
  [ -f "${file}" ] || die "no such snapshot: ${file}"
  [ -f "${file}.sha256" ] || die "no checksum beside ${file} - integrity cannot be established"
  local expected actual
  expected="$(cat "${file}.sha256")"
  actual="$(shasum -a 256 "${file}" | awk '{print $1}')"
  [ "${expected}" = "${actual}" ] || die "CHECKSUM MISMATCH - this snapshot is corrupt and must not be restored
     expected ${expected}
     actual   ${actual}"
  info "checksum OK: ${file}"
}

cmd_restore() {
  local file="${1:-}" confirmed="${2:-}"
  [ -n "${file}" ] || die "usage: snapshot.sh restore <file> --yes"
  [ -f "${file}" ] || die "no such snapshot: ${file}"
  if [ "${confirmed}" != "--yes" ]; then
    cat >&2 <<EOF
Refusing to restore without explicit confirmation.

Restoring REPLACES the entire cluster: every secret, policy, token, and
tenant created since this snapshot was taken will be lost.

After restore, Vault seals and must be unsealed with the key shares that
were current WHEN THIS SNAPSHOT WAS TAKEN - not necessarily the ones in
.bootstrap/vault-init.json today.

Re-run with:  ./snapshot.sh restore ${file} --yes
EOF
    exit 1
  fi
  [ -f "${file}.sha256" ] && cmd_verify "${file}"
  info "restoring ${file} - this replaces all cluster data"
  curl -sf --max-time 300 \
    --header "X-Vault-Token: $(operator_token)" \
    --request POST \
    --data-binary "@${file}" \
    "${VAULT_ADDR}/v1/sys/storage/raft/snapshot-force" >/dev/null \
    || die "restore failed"
  info "restore submitted; Vault will seal"
  info "unseal with the key shares that were current when this snapshot was taken:"
  info "  ./setup.sh"
}

case "${1:-}" in
  take)    cmd_take ;;
  list)    cmd_list ;;
  verify)  shift; cmd_verify "${1:-}" ;;
  restore) shift; cmd_restore "${1:-}" "${2:-}" ;;
  -h|--help|"") sed -n '2,2p' "${BASH_SOURCE[0]}"; exit 0 ;;
  *) die "unknown subcommand: $1 (take, list, verify, restore)" ;;
esac
