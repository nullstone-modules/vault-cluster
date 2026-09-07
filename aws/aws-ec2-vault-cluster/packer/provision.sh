#!/usr/bin/env bash
set -euo pipefail

if ! id -u vault >/dev/null 2>&1; then
  useradd --system --home /opt/vault --shell /sbin/nologin vault
fi

install -d -m 0750 -o vault -g vault /opt/vault /opt/vault/data /opt/vault/audit /etc/vault.d

dnf install -y unzip
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "https://releases.hashicorp.com/vault/${VAULT_VERSION}/vault_${VAULT_VERSION}_linux_arm64.zip" -o "$tmp/vault.zip"
unzip -o "$tmp/vault.zip" -d "$tmp"
install -m 0755 "$tmp/vault" /usr/local/bin/vault
install -m 0755 /tmp/vault-utils /usr/local/bin/vault-utils
/usr/local/bin/vault version
/usr/local/bin/vault-utils >/dev/null || true
