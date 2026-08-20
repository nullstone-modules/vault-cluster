#!/usr/bin/env bash
# shellcheck shell=bash

REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
CONFIG_DIR="${REPO_ROOT}/config"
TESTS_DIR="${REPO_ROOT}/tests"
POLICY_TEMPLATE_DIR="${CONFIG_DIR}/policies/templates"
RENDERED_POLICY_DIR="${CONFIG_DIR}/policies/rendered"

export REPO_ROOT CONFIG_DIR TESTS_DIR POLICY_TEMPLATE_DIR RENDERED_POLICY_DIR

info() { printf '[config] %s\n'        "$*" >&2; }
warn() { printf '[config] WARN: %s\n'  "$*" >&2; }
die()  { printf '[config] ERROR: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  local missing=0 c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || { warn "required command not found: $c"; missing=1; }
  done
  [ "$missing" -eq 0 ] || die "install the missing dependencies listed above, then retry"
}

platform_defaults() {
  : "${KV_MOUNT:=kv}"
  : "${TENANT_PREFIX:=customers}"
  : "${DATABASE_MOUNT:=database}"
  : "${AUTH_MOUNT:=approle}"
  : "${DEFAULT_TOKEN_TTL:=1h}"
  : "${MAX_TOKEN_TTL:=24h}"
  : "${DATABASE_DEFAULT_TTL:=1h}"
  : "${DATABASE_MAX_TTL:=24h}"
  : "${ENABLE_AUDIT:=true}"
  : "${ENABLE_DYNAMIC_CREDENTIALS:=false}"
  export KV_MOUNT TENANT_PREFIX DATABASE_MOUNT AUTH_MOUNT \
         DEFAULT_TOKEN_TTL MAX_TOKEN_TTL DATABASE_DEFAULT_TTL DATABASE_MAX_TTL \
         ENABLE_AUDIT ENABLE_DYNAMIC_CREDENTIALS
}

credentials_enabled() {
  [ "${ENABLE_DYNAMIC_CREDENTIALS:-false}" = "true" ]
}

tenant_policy_reader()   { printf 'tenant-%s-reader' "$1"; }
tenant_policy_writer()   { printf 'tenant-%s-writer' "$1"; }
tenant_policy_database() { printf 'tenant-%s-database' "$1"; }
tenant_role_reader()     { printf 'tenant-%s-reader' "$1"; }
tenant_role_writer()     { printf 'tenant-%s-writer' "$1"; }
tenant_kv_data_path()    { printf '%s/data/%s/%s' "${KV_MOUNT}" "${TENANT_PREFIX}" "$1"; }
tenant_kv_meta_path()    { printf '%s/metadata/%s/%s' "${KV_MOUNT}" "${TENANT_PREFIX}" "$1"; }
