data "aws_iam_policy_document" "assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "this" {
  name               = local.resource_name
  assume_role_policy = data.aws_iam_policy_document.assume.json
  tags               = local.tags
}

resource "aws_iam_instance_profile" "this" {
  name = local.resource_name
  role = aws_iam_role.this.name
  tags = local.tags
}

resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.this.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

data "aws_iam_policy_document" "this" {
  statement {
    sid    = "UnsealKey"
    effect = "Allow"
    actions = [
      "kms:Encrypt",
      "kms:Decrypt",
      "kms:DescribeKey",
    ]
    resources = [local.unseal_kms_key_arn]
  }

  dynamic "statement" {
    for_each = local.snapshot_kms_key_arn == "" ? [] : [local.snapshot_kms_key_arn]
    content {
      sid    = "SnapshotKey"
      effect = "Allow"
      actions = [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:DescribeKey",
      ]
      resources = [statement.value]
    }
  }

  statement {
    sid    = "SnapshotList"
    effect = "Allow"
    actions = [
      "s3:ListBucket",
    ]
    resources = [local.snapshot_bucket_arn]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = [local.snapshot_prefix, "${local.snapshot_prefix}/*"]
    }
  }

  statement {
    sid    = "SnapshotObjects"
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
    ]
    resources = ["${local.snapshot_bucket_arn}/${local.snapshot_prefix}/*"]
  }

  statement {
    sid    = "PlatformTokens"
    effect = "Allow"
    actions = [
      "secretsmanager:GetSecretValue",
      "secretsmanager:PutSecretValue",
    ]
    resources = [for s in aws_secretsmanager_secret.platform : s.arn]
  }

  statement {
    sid       = "RaftJoin"
    effect    = "Allow"
    actions   = ["ec2:DescribeInstances"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "this" {
  name   = local.resource_name
  role   = aws_iam_role.this.id
  policy = data.aws_iam_policy_document.this.json
}
