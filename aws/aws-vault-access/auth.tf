data "aws_iam_role" "app" {
  name = var.app_metadata["role_name"]
}

resource "aws_lambda_invocation" "aws_auth_role" {
  function_name   = local.admin_function_name
  lifecycle_scope = "CRUD"

  input = jsonencode({
    type = "aws_auth_role"
    data = {
      name                    = var.vault_role
      bound_iam_principal_arn = data.aws_iam_role.app.arn
      policies                = var.vault_policies
    }
  })
}
