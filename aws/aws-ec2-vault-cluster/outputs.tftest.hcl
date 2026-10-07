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
      arn = "arn:aws:iam::123456789012:role/vault-abcde"
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

run "uses_http_without_a_subdomain" {
  command = plan

  assert {
    condition     = output.vault_addr == "http://vault.internal:8200" && output.user_vault_addr == "" && output.tls_server_name == "" && output.db_hostname == "vault.internal" && output.db_port == 8200 && output.db_endpoint == "vault://vault.internal:8200" && output.private_urls == tolist(["http://vault.internal:8200/ui/"]) && length(output.public_urls) == 0
    error_message = "Without a subdomain the NLB has no TLS listener."
  }
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
    condition     = output.vault_addr == "https://vault.internal:8200" && output.user_vault_addr == "https://vault.acme.example.com:8200" && output.tls_server_name == "vault.acme.example.com" && output.db_hostname == "vault.acme.example.com" && output.db_endpoint == "vault://vault.acme.example.com:8200" && output.private_urls == tolist(["https://vault.internal:8200/ui/"]) && output.public_urls == tolist(["https://vault.acme.example.com:8200/ui/"])
    error_message = "With TLS, vault.internal needs the user-facing name for certificate verification."
  }
}

run "is_not_shared_outside_previews_shared" {
  command = plan

  assert {
    condition     = output.shared == false && strcontains(base64decode(aws_launch_template.this.user_data), "SHARED_ENVS=false")
    error_message = "A cluster in any env but previews-shared is unshared and tells its nodes so."
  }
}

run "is_shared_in_previews_shared" {
  command = plan

  override_data {
    target = data.ns_workspace.this
    values = {
      env_name = "previews-shared"
    }
  }

  assert {
    condition     = output.shared == true && strcontains(base64decode(aws_launch_template.this.user_data), "SHARED_ENVS=true")
    error_message = "A cluster in previews-shared is shared and its nodes scope tenants per env."
  }
}
