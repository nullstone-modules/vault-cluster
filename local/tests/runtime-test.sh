#!/usr/bin/env bash
# Local runtime: persistence, audit volume, loopback bind. Requires ./setup.sh.
set -uo pipefail

ADAPTER_TESTS_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../bootstrap/lib.sh
. "${ADAPTER_TESTS_DIR}/../bootstrap/lib.sh"

require_docker
require_cmd curl jq
load_env

PASSED=0
FAILED=0
ok()  { printf '    PASS  %s\n' "$1"; PASSED=$((PASSED + 1)); }
no()  { printf '    FAIL  %s\n' "$1"; [ -n "${2:-}" ] && printf '          %s\n' "$2"; FAILED=$((FAILED + 1)); }
sec() { printf '\n  %s\n' "$1"; }

printf '\nLocal runtime tests\n'

sec "Containers and health"

if compose ps --status running --format '{{.Service}}' 2>/dev/null | grep -q '^vault$'; then
  ok "vault container is running"
else
  no "vault container is running" "run ./bootstrap/up.sh first"
  printf '\n  %d passed, %d failed\n\n' "${PASSED}" "${FAILED}"
  exit 1
fi

HEALTH_STATE="$(docker inspect --format '{{.State.Health.Status}}' vault-cluster-vault 2>/dev/null || echo unknown)"
if [ "${HEALTH_STATE}" = "healthy" ]; then
  ok "compose healthcheck reports healthy"
else
  no "compose healthcheck reports healthy" "state was '${HEALTH_STATE}'"
fi

if compose ps --status running --format '{{.Service}}' 2>/dev/null | grep -qE '^(unseal|bootstrap)$'; then
  no "bootstrap is not a long-running service" \
     "unseal sidecar / bootstrap should not stay up; re-run via ./setup.sh"
else
  ok "bootstrap is not a long-running service"
fi

if [ "$(vault_health_code)" = "200" ]; then
  ok "Vault reports initialized and unsealed"
else
  no "Vault reports initialized and unsealed" \
     "health code $(vault_health_code) - run ./setup.sh"
fi

sec "Dev mode is not in use"

if compose config 2>/dev/null | grep -qE '(-dev\b|VAULT_DEV_ROOT_TOKEN_ID)'; then
  no "no dev-mode configuration present" "found dev-mode settings in the Compose configuration"
else
  ok "no dev-mode configuration present"
fi

SEAL_TYPE="$(curl -s "${VAULT_ADDR}/v1/sys/seal-status" | jq -r '.type // "unknown"')"
if [ "${SEAL_TYPE}" = "shamir" ]; then
  ok "seal type is shamir (real unseal flow is exercised)"
else
  no "seal type is shamir" "got '${SEAL_TYPE}'"
fi

STORAGE_TYPE="$(curl -s "${VAULT_ADDR}/v1/sys/seal-status" | jq -r '.storage_type // "unknown"')"
if [ "${STORAGE_TYPE}" = "raft" ]; then
  ok "storage backend is raft"
else
  no "storage backend is raft" "got '${STORAGE_TYPE}'"
fi

sec "Storage separation"

RAFT_VOL="$(docker inspect --format \
  '{{range .Mounts}}{{if eq .Destination "/vault/file"}}{{.Name}}{{end}}{{end}}' vault-cluster-vault 2>/dev/null)"
AUDIT_VOL="$(docker inspect --format \
  '{{range .Mounts}}{{if eq .Destination "/vault/logs"}}{{.Name}}{{end}}{{end}}' vault-cluster-vault 2>/dev/null)"

if [ -n "${RAFT_VOL}" ] && [ -n "${AUDIT_VOL}" ]; then
  ok "both raft and audit volumes are mounted (${RAFT_VOL}, ${AUDIT_VOL})"
  if [ "${RAFT_VOL}" != "${AUDIT_VOL}" ]; then
    ok "audit storage is a separate volume from raft storage"
  else
    no "audit storage is a separate volume from raft storage" \
       "both use '${RAFT_VOL}' - audit growth can corrupt or fill Raft storage"
  fi
else
  no "both raft and audit volumes are mounted" "raft='${RAFT_VOL}' audit='${AUDIT_VOL}'"
fi

if docker exec vault-cluster-vault sh -c 'test -s /vault/logs/audit.log' 2>/dev/null; then
  ok "audit log exists and is non-empty"
else
  no "audit log exists and is non-empty" "no audit output at /vault/logs/audit.log"
fi

if docker exec vault-cluster-vault sh -c 'ls /vault/file/vault.db' >/dev/null 2>&1; then
  ok "raft storage is populated"
else
  no "raft storage is populated" "no vault.db under /vault/file"
fi

sec "Network exposure is loopback only"

PORT_BINDING="$(docker inspect --format \
  '{{range $p, $conf := .NetworkSettings.Ports}}{{range $conf}}{{$p}}={{.HostIp}} {{end}}{{end}}' \
  vault-cluster-vault 2>/dev/null)"

if printf '%s' "${PORT_BINDING}" | grep -q '127.0.0.1'; then
  ok "Vault port is published to 127.0.0.1 only"
else
  no "Vault port is published to 127.0.0.1 only" \
     "binding was '${PORT_BINDING}' - a non-loopback binding exposes a plaintext Vault"
fi

sec "Data persists across a restart"

PROBE_PATH="${KV_MOUNT:-kv}/data/${TENANT_PREFIX:-customers}/tenant-a/runtime-persistence-probe"
PROBE_VALUE="FAKE-persistence-probe-$(date +%s)"
OPERATOR_TOKEN_FILE="${BOOTSTRAP_DIR}/provisioning.token"

if [ ! -f "${OPERATOR_TOKEN_FILE}" ]; then
  no "bootstrap tokens are present" "run ./setup.sh"
else
  export VAULT_TOKEN; VAULT_TOKEN="$(cat "${OPERATOR_TOKEN_FILE}")"
  # shellcheck source=/dev/null
  . "${REPO_ROOT}/tests/conformance/common/setup.sh"

  if WRITER_TOKEN="$(approle_login "tenant-tenant-a-writer" 2>/dev/null)"; then
    if curl -s -o /dev/null -w '%{http_code}' \
        --header "X-Vault-Token: ${WRITER_TOKEN}" \
        --request POST \
        --data "$(jq -nc --arg v "${PROBE_VALUE}" '{data: {probe: $v}}')" \
        "${VAULT_ADDR}/v1/${PROBE_PATH}" | grep -q '^2'; then
      ok "probe secret written before restart"

      info "restarting Vault (this takes a few seconds)"
      compose restart vault >/dev/null 2>&1
      wait_for_vault 60 >/dev/null 2>&1

      if vault_is_sealed; then
        ok "Vault resealed after process restart (expected for Shamir)"
      else
        no "Vault resealed after process restart" "process restart left Vault unsealed"
      fi

      info "re-running one-shot bootstrap to unseal"
      if compose run --rm --no-deps bootstrap >/dev/null; then
        ok "one-shot bootstrap unsealed Vault after restart"
      else
        no "one-shot bootstrap unsealed Vault after restart" "bootstrap failed"
      fi

      if vault_is_sealed; then
        no "Vault is unsealed after bootstrap" "still sealed"
      else
        ok "Vault is unsealed after bootstrap"

        READ_VALUE="$(curl -s --header "X-Vault-Token: ${WRITER_TOKEN}" \
          "${VAULT_ADDR}/v1/${PROBE_PATH}" | jq -r '.data.data.probe // empty')"

        if [ "${READ_VALUE}" = "${PROBE_VALUE}" ]; then
          ok "probe secret survived the restart intact"
        else
          no "probe secret survived the restart intact" \
             "expected '${PROBE_VALUE}', got '${READ_VALUE}' - storage is not persistent"
        fi

        if curl -s -o /dev/null -w '%{http_code}' \
             --header "X-Vault-Token: ${WRITER_TOKEN}" \
             "${VAULT_ADDR}/v1/auth/token/lookup-self" | grep -q '^2'; then
          ok "pre-restart token is still valid (token storage persisted)"
        else
          no "pre-restart token is still valid" "the token did not survive the restart"
        fi
      fi
    else
      no "probe secret written before restart" "write was refused"
    fi
  else
    no "tenant AppRole login for the persistence probe" \
       "has tenant-a been onboarded?"
  fi
fi

sec "Compose configuration"

if compose config 2>/dev/null | grep -qE 'image:.*@sha256:'; then
  ok "images are digest-pinned"
else
  no "images are digest-pinned" "a floating tag lets the environment change without a commit"
fi

if compose config 2>/dev/null | grep -qE 'image:.*:latest'; then
  no "no image uses the 'latest' tag" "found a ':latest' reference"
else
  ok "no image uses the 'latest' tag"
fi

printf '\n  %d passed, %d failed\n\n' "${PASSED}" "${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
