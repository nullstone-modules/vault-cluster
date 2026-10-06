data "aws_iam_role" "app" {
  name = var.app_metadata["role_name"]
}

# The cluster function writes one Vault auth role for this app's IAM role on the aws auth mount.
resource "aws_lambda_invocation" "vault_role" {
  function_name   = local.admin_function_name
  lifecycle_scope = "CRUD"

  input = jsonencode({
    type = "vault_role"
    data = {
      name      = local.role_name
      method    = "aws"
      principal = data.aws_iam_role.app.arn
      policies  = ["apps-${var.access}"]
    }
  })
}
