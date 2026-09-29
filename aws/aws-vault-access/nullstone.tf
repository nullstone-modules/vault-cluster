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
  role_name = var.role_name != "" ? var.role_name : "${data.ns_workspace.this.block_name}-${random_string.resource_suffix.result}"
}
