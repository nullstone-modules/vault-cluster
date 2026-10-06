module "user_cert" {
  source  = "nullstone-modules/sslcert/aws"
  version = "~> 0.3.0"
  enabled = local.subdomain_zone_id != "" && !local.subdomain_has_certificate

  providers = {
    aws        = aws
    aws.domain = aws
  }

  domain = {
    name    = local.subdomain_name != "" ? local.subdomain_name : local.vault_fqdn
    zone_id = local.subdomain_zone_id != "" ? local.subdomain_zone_id : local.internal_zone_id
  }

  tags = local.tags
}

locals {
  user_certificate_arn = local.subdomain_has_certificate ? local.subdomain_certificate_arn : module.user_cert.certificate_arn
  nlb_certificate_arn  = local.user_certificate_arn
  # Known at plan time: a connected subdomain always yields a certificate (its own or module.user_cert).
  nlb_tls = local.subdomain_zone_id != ""
}
