resource "aws_security_group" "nlb" {
  name   = "${local.resource_name}/nlb"
  vpc_id = local.vpc_id
  tags   = merge(local.tags, { Name = "${local.resource_name}/nlb" })
}

resource "aws_security_group" "nodes" {
  name   = "${local.resource_name}/nodes"
  vpc_id = local.vpc_id
  tags   = merge(local.tags, { Name = "${local.resource_name}/nodes" })
}

resource "aws_security_group_rule" "nlb_api_from_vpc" {
  security_group_id = aws_security_group.nlb.id
  type              = "ingress"
  protocol          = "tcp"
  from_port         = local.vault_api_port
  to_port           = local.vault_api_port
  cidr_blocks       = [local.vpc_cidr]
}

resource "aws_security_group_rule" "nlb_to_api" {
  security_group_id        = aws_security_group.nlb.id
  type                     = "egress"
  protocol                 = "tcp"
  from_port                = local.vault_api_port
  to_port                  = local.vault_api_port
  source_security_group_id = aws_security_group.nodes.id
}

resource "aws_security_group_rule" "nlb_to_health" {
  security_group_id        = aws_security_group.nlb.id
  type                     = "egress"
  protocol                 = "tcp"
  from_port                = local.vault_health_port
  to_port                  = local.vault_health_port
  source_security_group_id = aws_security_group.nodes.id
}

resource "aws_security_group_rule" "nodes_api_from_nlb" {
  security_group_id        = aws_security_group.nodes.id
  type                     = "ingress"
  protocol                 = "tcp"
  from_port                = local.vault_api_port
  to_port                  = local.vault_api_port
  source_security_group_id = aws_security_group.nlb.id
}

resource "aws_security_group_rule" "nodes_health_from_nlb" {
  security_group_id        = aws_security_group.nodes.id
  type                     = "ingress"
  protocol                 = "tcp"
  from_port                = local.vault_health_port
  to_port                  = local.vault_health_port
  source_security_group_id = aws_security_group.nlb.id
}

resource "aws_security_group_rule" "nodes_raft" {
  security_group_id        = aws_security_group.nodes.id
  type                     = "ingress"
  protocol                 = "tcp"
  from_port                = local.vault_cluster_port
  to_port                  = local.vault_cluster_port
  source_security_group_id = aws_security_group.nodes.id
}

resource "aws_security_group_rule" "nodes_https" {
  security_group_id = aws_security_group.nodes.id
  type              = "egress"
  protocol          = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_blocks       = ["0.0.0.0/0"]
}

resource "aws_security_group_rule" "nodes_raft_egress" {
  security_group_id        = aws_security_group.nodes.id
  type                     = "egress"
  protocol                 = "tcp"
  from_port                = local.vault_cluster_port
  to_port                  = local.vault_cluster_port
  source_security_group_id = aws_security_group.nodes.id
}
