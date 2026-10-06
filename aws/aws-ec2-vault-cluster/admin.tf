module "vault_admin" {
  source  = "api.nullstone.io/nullstone/aws-vault-admin/aws"
  version = "~> 0.1.1"

  name             = "${local.resource_name}-aws-auth"
  tags             = local.tags
  vault_addr       = local.vault_addr
  tls_server_name  = local.tls_server_name
  vault_port       = local.vault_api_port
  token_secret_arn = aws_secretsmanager_secret.platform["aws-auth"].arn
  network = {
    vpc_id                  = local.vpc_id
    vault_security_group_id = aws_security_group.nlb.id
    subnet_ids              = local.private_subnet_ids
  }
}
