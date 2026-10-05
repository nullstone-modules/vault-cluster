mock_provider "random" {}

mock_provider "ns" {
  mock_data "ns_workspace" {
    defaults = {
      stack_name = "core"
      env_name   = "prod"
      block_ref  = "vault"
      block_name = "vault"
      aws_tags   = {}
    }
  }

  mock_data "ns_connection" {
    defaults = {
      outputs = {
        vpc_id             = "vpc-1"
        vpc_cidr           = "10.0.0.0/16"
        private_subnet_ids = ["subnet-1"]
        internal_zone_id   = "Z1"
        db_arn             = "arn:aws:s3:::snapshots"
        kms_key_arn        = "arn:aws:kms:us-east-1:123456789012:key/1"
      }
    }
  }
}

mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "123456789012"
    }
  }

  mock_data "aws_partition" {
    defaults = {
      partition = "aws"
    }
  }

  mock_data "aws_region" {
    defaults = {
      name   = "us-east-1"
      region = "us-east-1"
    }
  }

  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{}"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::123456789012:role/vault-abcde-admin"
    }
  }

  mock_resource "aws_launch_template" {
    defaults = {
      id = "lt-0123456789abcdef0"
    }
  }

  mock_resource "aws_lb" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/net/vault/0123456789abcdef"
    }
  }

  mock_resource "aws_lb_target_group" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/vault-api/0123456789abcdef"
    }
  }

  mock_resource "aws_route53_record" {
    defaults = {
      fqdn = "vault.internal"
    }
  }
}

variables {
  ami = "ami-1"
}

run "binds_groups_and_extra_principals" {
  command = plan

  variables {
    admin_principals = {
      sso = {
        principal_arn = "arn:aws:iam::123456789012:role/aws-reserved/sso.amazonaws.com/AWSReservedSSO_VaultAdmin_*"
        access        = ["tenants", "operator", "tenants"]
      }
    }
  }

  assert {
    condition     = [for b in local.admin_bindings : b.name] == ["admin-operator", "admin-tenants", "admin-sso"]
    error_message = "Each access level and each extra principal must get one admin-* role."
  }

  assert {
    condition     = local.admin_bindings[2].access == tolist(["operator", "tenants"])
    error_message = "Access must be deduplicated."
  }

  assert {
    condition     = strcontains(base64decode(aws_launch_template.this.user_data), "AWSReservedSSO_VaultAdmin_*") && strcontains(base64decode(aws_launch_template.this.user_data), "VAULT_ADMINS_FILE=/etc/vault.d/admins.json")
    error_message = "Bindings must reach the nodes through user-data."
  }

  assert {
    condition     = keys(aws_iam_group.admin) == ["operator", "tenants"]
    error_message = "One IAM group per access level."
  }

  assert {
    condition     = startswith(aws_iam_group.admin["tenants"].name, "core-prod-vault-") && endswith(aws_iam_group.admin["tenants"].name, "-vault-tenants")
    error_message = "Group names must carry stack, env, block ref, and the resource suffix."
  }
}

run "uses_http_without_a_subdomain" {
  command = plan

  assert {
    condition     = output.vault_addr == "http://vault.internal:8200" && output.user_vault_addr == "" && output.tls_server_name == ""
    error_message = "Without a subdomain the NLB has no TLS listener."
  }
}

run "rejects_reserved_key" {
  command = plan

  variables {
    admin_principals = {
      operator = { principal_arn = "arn:aws:iam::123456789012:user/brad", access = ["operator"] }
    }
  }

  expect_failures = [var.admin_principals]
}

run "rejects_invalid_key" {
  command = plan

  variables {
    admin_principals = {
      "Brad Smith" = { principal_arn = "arn:aws:iam::123456789012:user/brad", access = ["operator"] }
    }
  }

  expect_failures = [var.admin_principals]
}

run "rejects_group_arn" {
  command = plan

  variables {
    admin_principals = {
      admins = { principal_arn = "arn:aws:iam::123456789012:group/admins", access = ["tenants"] }
    }
  }

  expect_failures = [var.admin_principals]
}

run "rejects_account_wildcard" {
  command = plan

  variables {
    admin_principals = {
      everyone = { principal_arn = "arn:aws:iam::123456789012:*", access = ["tenants"] }
    }
  }

  expect_failures = [var.admin_principals]
}

run "rejects_unknown_access" {
  command = plan

  variables {
    admin_principals = {
      brad = { principal_arn = "arn:aws:iam::123456789012:user/brad", access = ["root"] }
    }
  }

  expect_failures = [var.admin_principals]
}

run "rejects_empty_access" {
  command = plan

  variables {
    admin_principals = {
      brad = { principal_arn = "arn:aws:iam::123456789012:user/brad", access = [] }
    }
  }

  expect_failures = [var.admin_principals]
}

run "rejects_duplicate_principal" {
  command = plan

  variables {
    admin_principals = {
      a = { principal_arn = "arn:aws:iam::123456789012:user/brad", access = ["tenants"] }
      b = { principal_arn = "arn:aws:iam::123456789012:user/brad", access = ["operator"] }
    }
  }

  expect_failures = [var.admin_principals]
}

run "uses_https_with_a_subdomain" {
  command = plan

  override_data {
    target = data.ns_connection.subdomain
    values = {
      outputs = {
        fqdn            = "vault.acme.example.com."
        zone_id         = "Z2"
        certificate_arn = "arn:aws:acm:us-east-1:123456789012:certificate/1"
      }
    }
  }

  override_resource {
    target = aws_route53_record.user
    values = {
      fqdn = "vault.acme.example.com"
    }
  }

  assert {
    condition     = output.vault_addr == "https://vault.internal:8200" && output.user_vault_addr == "https://vault.acme.example.com:8200" && output.tls_server_name == "vault.acme.example.com"
    error_message = "With TLS, vault.internal needs the user-facing name for certificate verification."
  }
}

run "truncates_long_group_names" {
  command = plan

  override_data {
    target = data.ns_workspace.this
    values = {
      stack_name = "a-very-long-stack-name-that-keeps-going-and-going-and-going"
      env_name   = "an environment with spaces and a very long name that also keeps going"
      block_ref  = "vault"
      block_name = "vault"
      aws_tags   = {}
    }
  }

  assert {
    condition     = alltrue([for g in aws_iam_group.admin : length(g.name) <= 128 && can(regex("^[A-Za-z0-9+=,.@_-]+$", g.name))])
    error_message = "Group names must fit IAM's 128-character limit and character set."
  }
}
