locals {
  platform_secret_names = toset(["init", "provisioning", "operator"])
}

resource "aws_secretsmanager_secret" "platform" {
  for_each = local.platform_secret_names

  name_prefix             = "${local.block_name}/vault/${each.key}/"
  recovery_window_in_days = 0
  tags                    = local.tags
}
