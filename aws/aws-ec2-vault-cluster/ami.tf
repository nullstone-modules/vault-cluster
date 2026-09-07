data "aws_ami" "vault" {
  count       = var.ami == "" ? 1 : 0
  most_recent = true
  owners      = ["self"]

  filter {
    name   = "name"
    values = ["nullstone-vault-*"]
  }

  filter {
    name   = "architecture"
    values = ["arm64"]
  }

  filter {
    name   = "state"
    values = ["available"]
  }
}

locals {
  ami = var.ami != "" ? var.ami : data.aws_ami.vault[0].id
}
