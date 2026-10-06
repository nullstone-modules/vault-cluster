data "aws_ami" "vault" {
  count       = var.ami == "" ? 1 : 0
  most_recent = true
  owners      = [var.ami_owner]

  # Tags are invisible to other accounts; the bake names the image nullstone-vault-<timestamp>.
  filter {
    name   = "name"
    values = ["nullstone-vault-*"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }

  filter {
    name   = "state"
    values = ["available"]
  }
}

locals {
  ami = var.ami != "" ? var.ami : data.aws_ami.vault[0].id
}
