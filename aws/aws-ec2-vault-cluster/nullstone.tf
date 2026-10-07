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

# A cluster launched in the previews-shared env serves every preview env and keeps their secrets apart
# (envs/<env>/customers/<tenant>). Anywhere else it is one env's cluster with the unscoped layout.
locals {
  shared_env_name = "previews-shared"
  shared          = data.ns_workspace.this.env_name == local.shared_env_name
}
