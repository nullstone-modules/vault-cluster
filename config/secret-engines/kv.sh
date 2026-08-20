#!/usr/bin/env bash
# Mount KV v2.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../scripts/lib/common.sh
. "${SCRIPT_DIR}/../../scripts/lib/common.sh"
# shellcheck source=../../scripts/lib/vault.sh
. "${SCRIPT_DIR}/../../scripts/lib/vault.sh"

platform_defaults
vault_require_env

if vault_mount_exists "${KV_MOUNT}"; then
  info "KV mount '${KV_MOUNT}/' already exists"
else
  info "mounting KV v2 at '${KV_MOUNT}/'"
  vault_must POST "sys/mounts/${KV_MOUNT}" "$(jq -nc \
    '{
       type: "kv",
       description: "Multi-tenant secret store",
       options: { version: "2" }
     }')"
fi

# Version 2 is the whole basis of the metadata/data path split the policies
# depend on. A KV v1 mount here would make every tenant policy match nothing, so
# verify rather than assume, since a pre-existing mount was not necessarily
# created by this script.
vault_must GET "sys/mounts/${KV_MOUNT}/tune"
if ! jq -e '.data.options.version == "2"' >/dev/null 2>&1 <<<"$(vault_body)"; then
  die "'${KV_MOUNT}/' is not KV v2 (sys/mounts/${KV_MOUNT}/tune options.version != 2).
     Every tenant policy in this platform targets ${KV_MOUNT}/data/... and
     ${KV_MOUNT}/metadata/..., which do not exist on v1, so they would apply
     cleanly and grant nothing."
fi

info "configuring KV v2 defaults"

# max_versions bounds unbounded version growth; a rotated-hourly secret would
# otherwise accumulate forever. 10 keeps enough history for `kv rollback` to be
# a usable mitigation when a rotation breaks a consumer.
vault_must POST "${KV_MOUNT}/config" "$(jq -nc \
  --argjson max "${KV_MAX_VERSIONS:-10}" \
  '{
     max_versions: $max,
     cas_required: false,
     delete_version_after: "0s"
   }')"

info "KV v2 ready at '${KV_MOUNT}/' (max_versions=${KV_MAX_VERSIONS:-10})"
