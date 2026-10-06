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
        vault_addr            = "http://vault.internal:8200"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8200"
        admin_function_name   = "vault-apps-auth"
      }
    }
  }

  assert {
    condition     = output.env == [{ name = "VAULT_ADDR", value = "http://vault.internal:8200" }, { name = "VAULT_ROLE", value = "billing" }, { name = "VAULT_TLS_SERVER_NAME", value = "" }, { name = "VAULT_TENANT_MOUNT", value = "approle-reader" }]
    error_message = "The app must receive VAULT_ADDR, the capability role, an empty TLS name without TLS, and its tenant mount."
  }

  assert {
    condition     = jsondecode(aws_lambda_invocation.vault_role.input).data.principal == "arn:aws:iam::123456789012:role/app"
    error_message = "The function must bind only the app IAM role."
  }

  assert {
    condition     = jsondecode(aws_lambda_invocation.vault_role.input).data.policies == ["apps-reader"]
    error_message = "A reader app gets only the apps-reader broker policy."
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
        vault_addr            = "http://vault.internal:8443"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8443"
        admin_function_name   = "vault-apps-auth"
      }
    }
  }

  assert {
    condition     = output.env[0].value == "http://vault.internal:8443" && aws_security_group_rule.app_to_vault.from_port == 8443
    error_message = "VAULT_ADDR and the security group rules must use vault_api_port from the cluster."
  }
}

run "writer_access_selects_the_writer_mount" {
  command = plan

  variables {
    access = "writer"
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

  assert {
    condition     = jsondecode(aws_lambda_invocation.vault_role.input).data.policies == ["apps-writer"] && output.env[3].value == "approle-writer"
    error_message = "A writer app gets apps-writer and logs in on approle-writer."
  }
}

run "rejects_unknown_access" {
  command = plan

  variables {
    access = "operator"
  }

  override_data {
    target = data.ns_connection.vault
    values = {
      outputs = {
        vault_fqdn            = "vault.internal"
        vault_addr            = "http://vault.internal:8200"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8200"
        admin_function_name   = "vault-apps-auth"
      }
    }
  }

  expect_failures = [
    var.access,
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
        vault_addr            = "http://vault.internal:8200"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8200"
        admin_function_name   = "vault-apps-auth"
      }
    }
  }

  expect_failures = [
    var.role_name,
  ]
}

run "uses_the_cluster_addr_and_tls_name" {
  command = plan

  override_data {
    target = data.ns_connection.vault
    values = {
      outputs = {
        vault_fqdn            = "vault.internal"
        vault_addr            = "https://vault.internal:8200"
        tls_server_name       = "vault.acme.example.com"
        nlb_security_group_id = "sg-nlb"
        vault_api_port        = "8200"
        admin_function_name   = "vault-aws-auth"
      }
    }
  }

  assert {
    condition = output.env == [
      { name = "VAULT_ADDR", value = "https://vault.internal:8200" },
      { name = "VAULT_ROLE", value = "billing" },
      { name = "VAULT_TLS_SERVER_NAME", value = "vault.acme.example.com" },
      { name = "VAULT_TENANT_MOUNT", value = "approle-reader" },
    ]
    error_message = "A TLS cluster must give the app its https address and the certificate name."
  }
}
