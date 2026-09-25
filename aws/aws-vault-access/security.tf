resource "aws_security_group_rule" "app_to_vault" {
  security_group_id        = local.security_group_id
  type                     = "egress"
  protocol                 = "tcp"
  from_port                = local.vault_api_port
  to_port                  = local.vault_api_port
  source_security_group_id = local.nlb_security_group_id
}

resource "aws_security_group_rule" "vault_from_app" {
  security_group_id        = local.nlb_security_group_id
  type                     = "ingress"
  protocol                 = "tcp"
  from_port                = local.vault_api_port
  to_port                  = local.vault_api_port
  source_security_group_id = local.security_group_id
}
