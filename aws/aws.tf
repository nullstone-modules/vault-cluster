provider "aws" {
  default_tags {
    tags = local.tags
  }
}

data "aws_region" "this" {}

data "aws_caller_identity" "this" {}
