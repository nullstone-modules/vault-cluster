locals {
  user_data = templatefile("${path.module}/templates/user-data.sh.tpl", {
    aws_region              = data.aws_region.this.region
    unseal_kms_key_arn      = local.unseal_kms_key_arn
    vault_cluster_tag_key   = local.vault_cluster_tag_key
    vault_cluster_tag_value = local.vault_cluster_tag_value
    vault_api_port          = local.vault_api_port
    vault_cluster_port      = local.vault_cluster_port
    vault_health_port       = local.vault_health_port
    init_secret_arn         = aws_secretsmanager_secret.platform["init"].arn
    provisioning_secret_arn = aws_secretsmanager_secret.platform["provisioning"].arn
    operator_secret_arn     = aws_secretsmanager_secret.platform["operator"].arn
    snapshot_bucket_name    = local.snapshot_bucket_name
    snapshot_prefix         = local.snapshot_prefix
    backup_schedule         = var.backup_schedule
    audit_log_path          = "/opt/vault/audit/audit.log"
  })
}
