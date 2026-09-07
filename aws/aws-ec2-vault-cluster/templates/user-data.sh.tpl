#!/usr/bin/env bash
set -euo pipefail

imds_token() {
  curl -fsS -X PUT "http://169.254.169.254/latest/api/token" \
    -H "X-aws-ec2-metadata-token-ttl-seconds: 21600"
}

imds() {
  curl -fsS -H "X-aws-ec2-metadata-token: $1" "http://169.254.169.254/latest/meta-data/$2"
}

TOKEN="$(imds_token)"
INSTANCE_ID="$(imds "$TOKEN" instance-id)"
PRIVATE_IP="$(imds "$TOKEN" local-ipv4)"

install -d -m 0750 -o vault -g vault /opt/vault/data /opt/vault/audit /etc/vault.d

cat >/etc/vault.d/vault.hcl <<HCL
cluster_name = "${vault_cluster_tag_value}"
ui           = false

storage "raft" {
  path    = "/opt/vault/data"
  node_id = "$${INSTANCE_ID}"

  retry_join {
    auto_join        = "provider=aws region=${aws_region} tag_key=${vault_cluster_tag_key} tag_value=${vault_cluster_tag_value} addr_type=private_v4"
    auto_join_scheme = "http"
  }
}

listener "tcp" {
  address         = "0.0.0.0:${vault_api_port}"
  cluster_address = "0.0.0.0:${vault_cluster_port}"
  tls_disable     = 1
}

seal "awskms" {
  region     = "${aws_region}"
  kms_key_id = "${unseal_kms_key_arn}"
}

api_addr     = "http://$${PRIVATE_IP}:${vault_api_port}"
cluster_addr = "http://$${PRIVATE_IP}:${vault_cluster_port}"

log_level  = "info"
log_format = "json"
HCL
chown vault:vault /etc/vault.d/vault.hcl
chmod 0640 /etc/vault.d/vault.hcl

cat >/etc/vault.d/vault-utils.env <<ENV
VAULT_ADDR=http://127.0.0.1:${vault_api_port}
VAULT_RAFT_NODE_ID=$${INSTANCE_ID}
VAULT_HEALTH_ADDR=:${vault_health_port}
VAULT_INIT_SECRET_ARN=${init_secret_arn}
VAULT_PROVISIONING_SECRET_ARN=${provisioning_secret_arn}
VAULT_OPERATOR_SECRET_ARN=${operator_secret_arn}
SNAPSHOT_BUCKET=${snapshot_bucket_name}
SNAPSHOT_PREFIX=${snapshot_prefix}
BACKUP_SCHEDULE=${backup_schedule}
AUDIT_LOG_PATH=${audit_log_path}
AWS_REGION=${aws_region}
ENV
chown vault:vault /etc/vault.d/vault-utils.env
chmod 0600 /etc/vault.d/vault-utils.env

cat >/etc/systemd/system/vault.service <<'UNIT'
[Unit]
Description=Vault
Requires=network-online.target
After=network-online.target
ConditionFileNotEmpty=/etc/vault.d/vault.hcl

[Service]
User=vault
Group=vault
ExecStart=/usr/local/bin/vault server -config=/etc/vault.d/vault.hcl
ExecReload=/bin/kill --signal HUP $MAINPID
KillMode=process
KillSignal=SIGINT
Restart=on-failure
LimitNOFILE=65536
LimitMEMLOCK=infinity
CapabilityBoundingSet=CAP_SYSLOG CAP_IPC_LOCK
AmbientCapabilities=CAP_IPC_LOCK

[Install]
WantedBy=multi-user.target
UNIT

cat >/etc/systemd/system/vault-bootstrap.service <<'UNIT'
[Unit]
Description=Vault bootstrap
After=vault.service
Requires=vault.service

[Service]
Type=oneshot
RemainAfterExit=yes
EnvironmentFile=/etc/vault.d/vault-utils.env
ExecStart=/usr/local/bin/vault-utils bootstrap aws
User=vault
Group=vault

[Install]
WantedBy=multi-user.target
UNIT

cat >/etc/systemd/system/vault-health.service <<'UNIT'
[Unit]
Description=Vault raft health
After=vault-bootstrap.service
Requires=vault-bootstrap.service

[Service]
EnvironmentFile=/etc/vault.d/vault-utils.env
ExecStart=/usr/local/bin/vault-utils health serve
Restart=always
RestartSec=5
User=vault
Group=vault

[Install]
WantedBy=multi-user.target
UNIT

cat >/etc/systemd/system/vault-snapshot.service <<'UNIT'
[Unit]
Description=Vault raft snapshots
After=vault-bootstrap.service
Requires=vault-bootstrap.service

[Service]
EnvironmentFile=/etc/vault.d/vault-utils.env
ExecStart=/usr/local/bin/vault-utils snapshot schedule
Restart=always
RestartSec=30
User=vault
Group=vault

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now amazon-ssm-agent
systemctl enable --now vault

ready=0
for _ in $(seq 1 60); do
  if curl -fsS --max-time 2 \
    "http://127.0.0.1:${vault_api_port}/v1/sys/health?standbyok=true&sealedcode=200&uninitcode=200" \
    >/dev/null; then
    ready=1
    break
  fi
  sleep 2
done
if [ "$ready" -ne 1 ]; then
  echo "vault did not become reachable" >&2
  exit 1
fi

systemctl start vault-bootstrap.service
systemctl enable --now vault-health.service
if [ -n "${backup_schedule}" ]; then
  systemctl enable --now vault-snapshot.service
fi
