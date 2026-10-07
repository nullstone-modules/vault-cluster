terraform {
  required_providers {
    ns = {
      source  = "nullstone-io/ns"
      version = "~> 0.11.0"
    }
    aws = {
      source = "hashicorp/aws"
    }
    random = {
      source = "hashicorp/random"
    }
  }
}

data "ns_workspace" "this" {}

resource "random_string" "resource_suffix" {
  length  = 5
  lower   = true
  upper   = false
  numeric = false
  special = false
}

locals {
  tags          = data.ns_workspace.this.aws_tags
  block_name    = data.ns_workspace.this.block_name
  resource_name = "${data.ns_workspace.this.block_ref}-${random_string.resource_suffix.result}"
}

data "ns_env" "this" {
  stack_id = data.ns_workspace.this.stack_id
  env_id   = data.ns_workspace.this.env_id
}

# A cluster launched in the stack's shared previews env serves every preview env and keeps their secrets
# apart (envs/<env>/customers/<tenant>). Anywhere else it is one env's cluster with the unscoped layout.
locals {
  shared = data.ns_env.this.type == "PreviewsSharedEnv"
}
