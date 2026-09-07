#!/usr/bin/env bash
set -euo pipefail

cat >/etc/vault.d/vault-utils.env <<ENV
VAULT_ADDR=http://127.0.0.1:8200
VAULT_HEALTH_ADDR=:8210
VAULT_INIT_SECRET_ARN=${init_secret_arn}
VAULT_PROVISIONING_SECRET_ARN=${provisioning_secret_arn}
VAULT_OPERATOR_SECRET_ARN=${operator_secret_arn}
SNAPSHOT_BUCKET=${snapshot_bucket_name}
SNAPSHOT_PREFIX=${snapshot_prefix}
BACKUP_SCHEDULE=${backup_schedule}
AUDIT_LOG_PATH=/opt/vault/audit/audit.log
AWS_REGION=${aws_region}
UNSEAL_KMS_KEY_ARN=${unseal_kms_key_arn}
VAULT_CLUSTER_TAG_KEY=${vault_cluster_tag_key}
VAULT_CLUSTER_TAG_VALUE=${vault_cluster_tag_value}
ENV
chown vault:vault /etc/vault.d/vault-utils.env
chmod 0600 /etc/vault.d/vault-utils.env

/usr/local/bin/vault-node-configure
