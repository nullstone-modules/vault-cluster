provider "aws" {
  default_tags {
    tags = local.tags
  }
}

data "aws_region" "this" {}
