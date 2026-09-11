module "cert" {
  source  = "nullstone-modules/sslcert/aws"
  version = "~> 0.3.0"

  providers = {
    aws        = aws
    aws.domain = aws
  }

  domain = {
    name    = local.vault_fqdn
    zone_id = local.internal_zone_id
  }

  tags = local.tags
}
