data "aws_caller_identity" "this" {}
data "aws_partition" "this" {}

locals {
  admin_access_levels = toset(["tenants", "operator"])

  # IAM groups are account-wide and block_ref is unique only within a stack. Stack and env make the name unique
  # within an org; the resource suffix covers other orgs in the same account. Only stack-env is truncated (128-char limit).
  admin_group_scope  = substr(replace("${data.ns_workspace.this.stack_name}-${data.ns_workspace.this.env_name}", "/[^A-Za-z0-9+=,.@_-]/", "-"), 0, 60)
  admin_group_prefix = "${local.admin_group_scope}-${local.resource_name}"

  # Nodes write these as admin-* AWS auth roles at boot and remove any admin-* role not listed.
  admin_bindings = concat(
    [for level in sort(tolist(local.admin_access_levels)) : {
      name          = "admin-${level}"
      principal_arn = aws_iam_role.admin[level].arn
      access        = [level]
    }],
    [for key in sort(keys(var.admin_principals)) : {
      name          = "admin-${key}"
      principal_arn = var.admin_principals[key].principal_arn
      access        = sort(distinct(var.admin_principals[key].access))
    }],
  )
}

# Group members assume the role with their IAM user name as the session name, so Vault's audit log names the person.
data "aws_iam_policy_document" "admin_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "AWS"
      identifiers = ["arn:${data.aws_partition.this.partition}:iam::${data.aws_caller_identity.this.account_id}:root"]
    }
    condition {
      test     = "StringEquals"
      variable = "sts:RoleSessionName"
      values   = ["$${aws:username}"]
    }
    dynamic "condition" {
      for_each = var.admin_require_mfa ? [true] : []
      content {
        test     = "Bool"
        variable = "aws:MultiFactorAuthPresent"
        values   = ["true"]
      }
    }
  }
}

# No permissions: the role only proves identity to Vault.
resource "aws_iam_role" "admin" {
  for_each = local.admin_access_levels

  name               = "${local.resource_name}-admin-${each.key}"
  assume_role_policy = data.aws_iam_policy_document.admin_assume.json
  tags               = local.tags
}

resource "aws_iam_group" "admin" {
  for_each = local.admin_access_levels

  name = "${local.admin_group_prefix}-vault-${each.key}"
}

resource "aws_iam_group_policy" "admin" {
  for_each = local.admin_access_levels

  name  = "assume-vault-${each.key}"
  group = aws_iam_group.admin[each.key].name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "sts:AssumeRole"
      Resource = aws_iam_role.admin[each.key].arn
    }]
  })
}
