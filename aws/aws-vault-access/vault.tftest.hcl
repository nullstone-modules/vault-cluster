mock_provider "ns" {}
mock_provider "aws" {}

variables {
  role_name = "billing"
  app_metadata = {
    security_group_id = "sg-app"
    role_name         = "app"
  }
}

override_data {
  target = data.aws_iam_role.app
  values = {
    arn = "arn:aws:iam::123456789012:role/app"
  }
}

run "injects_the_capability_role" {
  command = plan

  override_data {
    target = data.ns_connection.vault
    values = {
      outputs = {
        vault_fqdn            = "vault.internal"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8200"
        admin_function_name   = "vault-aws-auth"
      }
    }
  }

  assert {
    condition     = output.env == [{ name = "VAULT_ADDR", value = "http://vault.internal:8200" }, { name = "VAULT_ROLE", value = "billing" }]
    error_message = "The app must receive only VAULT_ADDR and the capability role."
  }

  assert {
    condition     = jsondecode(aws_lambda_invocation.aws_auth_role.input).data.bound_iam_principal_arn == "arn:aws:iam::123456789012:role/app"
    error_message = "The function must bind only the app IAM role."
  }

  assert {
    condition     = aws_security_group_rule.app_to_vault.from_port == 8200 && aws_security_group_rule.vault_from_app.source_security_group_id == "sg-app"
    error_message = "Security group rules must use the cluster API port and the app security group."
  }
}

run "uses_the_cluster_port" {
  command = plan

  override_data {
    target = data.ns_connection.vault
    values = {
      outputs = {
        vault_fqdn            = "vault.internal"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8443"
        admin_function_name   = "vault-aws-auth"
      }
    }
  }

  assert {
    condition     = output.env[0].value == "http://vault.internal:8443" && aws_security_group_rule.app_to_vault.from_port == 8443
    error_message = "VAULT_ADDR and the security group rules must use vault_api_port from the cluster."
  }
}

run "rejects_platform_policy" {
  command = plan

  variables {
    policies = ["operator"]
  }

  override_data {
    target = data.ns_connection.vault
    values = {
      outputs = {
        vault_fqdn            = "vault.internal"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8200"
        admin_function_name   = "vault-aws-auth"
      }
    }
  }

  expect_failures = [
    var.policies,
  ]
}

run "rejects_invalid_role" {
  command = plan

  variables {
    role_name = "other role"
  }

  override_data {
    target = data.ns_connection.vault
    values = {
      outputs = {
        vault_fqdn            = "vault.internal"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8200"
        admin_function_name   = "vault-aws-auth"
      }
    }
  }

  expect_failures = [
    var.role_name,
  ]
}
