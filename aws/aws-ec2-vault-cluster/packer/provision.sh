#!/usr/bin/env bash
set -euo pipefail

if ! id -u vault >/dev/null 2>&1; then
  useradd --system --home /opt/vault --shell /sbin/nologin vault
fi

install -d -m 0750 -o vault -g vault /opt/vault /opt/vault/data /opt/vault/audit /etc/vault.d

dnf install -y unzip
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "https://releases.hashicorp.com/vault/${VAULT_VERSION}/vault_${VAULT_VERSION}_linux_amd64.zip" -o "$tmp/vault.zip"
unzip -o "$tmp/vault.zip" -d "$tmp"
install -m 0755 "$tmp/vault" /usr/local/bin/vault
install -m 0755 /tmp/vault-utils /usr/local/bin/vault-utils
install -m 0640 -o vault -g vault /tmp/vault-image/vault.hcl /etc/vault.d/vault.hcl
install -m 0755 /tmp/vault-image/vault-node-configure /usr/local/bin/vault-node-configure
install -m 0644 /tmp/vault-image/vault.service /etc/systemd/system/vault.service
install -m 0644 /tmp/vault-image/vault-configure.service /etc/systemd/system/vault-configure.service
install -m 0644 /tmp/vault-image/vault-configure.path /etc/systemd/system/vault-configure.path
install -m 0644 /tmp/vault-image/vault-bootstrap.service /etc/systemd/system/vault-bootstrap.service
install -m 0644 /tmp/vault-image/vault-health.service /etc/systemd/system/vault-health.service
install -m 0644 /tmp/vault-image/vault-snapshot.service /etc/systemd/system/vault-snapshot.service

bash -n /usr/local/bin/vault-node-configure
/usr/local/bin/vault version
/usr/local/bin/vault-utils >/dev/null || true

systemctl daemon-reload
systemctl enable amazon-ssm-agent vault-configure.path vault.service
