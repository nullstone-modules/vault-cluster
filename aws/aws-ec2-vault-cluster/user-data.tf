locals {
  user_data = templatefile("${path.module}/templates/user-data.sh.tpl", {
    aws_region              = data.aws_region.this.region
    unseal_kms_key_arn      = local.unseal_kms_key_arn
    vault_cluster_tag_key   = local.vault_cluster_tag_key
    vault_cluster_tag_value = local.vault_cluster_tag_value
    init_secret_arn         = aws_secretsmanager_secret.platform["init"].arn
    provisioning_secret_arn = aws_secretsmanager_secret.platform["provisioning"].arn
    operator_secret_arn     = aws_secretsmanager_secret.platform["operator"].arn
    apps_auth_secret_arn    = aws_secretsmanager_secret.platform["apps-auth"].arn
    snapshot_bucket_name    = local.snapshot_bucket_name
    snapshot_prefix         = local.snapshot_prefix
    backup_schedule         = var.backup_schedule
    shared                  = local.shared
    # api_addr is the URL Vault puts in redirects and sys/leader. It must be the origin clients use:
    # the user-facing name when a subdomain is connected (the NLB certificate names only that host),
    # else vault.internal with the scheme the NLB listens on.
    vault_api_addr = local.user_vault_addr != "" ? local.user_vault_addr : local.vault_addr
  })
}
