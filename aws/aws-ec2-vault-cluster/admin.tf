data "archive_file" "aws_auth" {
  type        = "zip"
  source_file = "${path.module}/files/aws-auth/index.py"
  output_path = "${path.module}/files/aws-auth.zip"
}

resource "aws_security_group" "aws_auth" {
  name   = "${local.resource_name}/aws-auth"
  vpc_id = local.vpc_id
  tags   = merge(local.tags, { Name = "${local.resource_name}/aws-auth" })
}

resource "aws_security_group_rule" "aws_auth_to_vault" {
  security_group_id        = aws_security_group.aws_auth.id
  type                     = "egress"
  protocol                 = "tcp"
  from_port                = local.vault_api_port
  to_port                  = local.vault_api_port
  source_security_group_id = aws_security_group.nlb.id
}

resource "aws_security_group_rule" "aws_auth_to_secrets" {
  security_group_id = aws_security_group.aws_auth.id
  type              = "egress"
  protocol          = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_blocks       = ["0.0.0.0/0"]
}

resource "aws_security_group_rule" "nlb_from_aws_auth" {
  security_group_id        = aws_security_group.nlb.id
  type                     = "ingress"
  protocol                 = "tcp"
  from_port                = local.vault_api_port
  to_port                  = local.vault_api_port
  source_security_group_id = aws_security_group.aws_auth.id
}

resource "aws_iam_role" "aws_auth" {
  name = "${local.resource_name}-aws-auth"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
  tags = local.tags
}

resource "aws_iam_role_policy_attachment" "aws_auth_basic" {
  role       = aws_iam_role.aws_auth.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "aws_auth_vpc" {
  role       = aws_iam_role.aws_auth.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

resource "aws_iam_role_policy" "aws_auth" {
  role = aws_iam_role.aws_auth.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = aws_secretsmanager_secret.platform["aws-auth"].arn
    }]
  })
}

resource "aws_lambda_function" "aws_auth" {
  function_name    = "${local.resource_name}-aws-auth"
  role             = aws_iam_role.aws_auth.arn
  runtime          = "python3.12"
  handler          = "index.handler"
  filename         = data.archive_file.aws_auth.output_path
  source_code_hash = data.archive_file.aws_auth.output_base64sha256
  timeout          = 15
  tags             = local.tags

  environment {
    variables = {
      VAULT_ADDR            = "http://${local.vault_fqdn}:${local.vault_api_port}"
      VAULT_TOKEN_SECRET_ID = aws_secretsmanager_secret.platform["aws-auth"].arn
    }
  }

  vpc_config {
    subnet_ids         = local.private_subnet_ids
    security_group_ids = [aws_security_group.aws_auth.id]
  }
}
