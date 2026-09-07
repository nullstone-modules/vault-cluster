packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = ">= 1.3.0"
    }
  }
}

variable "region" {
  type = string
}

variable "vault_version" {
  type    = string
  default = "2.0.0"
}

source "amazon-ebs" "vault" {
  ami_name        = "nullstone-vault-{{timestamp}}"
  ami_description = "Vault CE and vault-utils for aws-ec2-vault-cluster"
  instance_type   = "t4g.micro"
  region          = var.region
  ssh_username    = "ec2-user"

  source_ami_filter {
    filters = {
      name                = "al2023-ami-*-kernel-6.1-arm64"
      architecture        = "arm64"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    owners      = ["amazon"]
    most_recent = true
  }

  tags = {
    Name = "nullstone-vault"
  }
}

build {
  sources = ["source.amazon-ebs.vault"]

  provisioner "file" {
    source      = "vault-utils"
    destination = "/tmp/vault-utils"
  }

  provisioner "shell" {
    environment_vars = [
      "VAULT_VERSION=${var.vault_version}",
    ]
    script = "${path.root}/provision.sh"
  }
}
