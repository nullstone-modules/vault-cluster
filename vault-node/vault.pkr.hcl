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
  default = "2.0.4"
}

# Regions the finished AMI is copied to. One bake, then copies, instead of a build per region.
variable "ami_regions" {
  type    = list(string)
  default = []
}

variable "ami_org_arns" {
  type    = list(string)
  default = []
}

variable "ami_groups" {
  type    = list(string)
  default = []
}

variable "subnet_id" {
  type        = string
  default     = null
  description = "Public subnet for the builder. GitHub bake sets this from the aws-packer-builder workspace."
}

source "amazon-ebs" "vault" {
  ami_name                    = "nullstone-vault-{{timestamp}}"
  ami_description             = "Vault node for self-hosting a vault cluster with nullstone vault-utils"
  instance_type               = "t3.micro"
  region                      = var.region
  ami_regions                 = var.ami_regions
  ami_org_arns                = var.ami_org_arns
  ami_groups                  = var.ami_groups
  ssh_username                = "ec2-user"
  subnet_id                   = var.subnet_id
  associate_public_ip_address = true

  # The name must not match al2023-ami-minimal-*: the minimal image has a 2 GiB root, and the Vault binary
  # does not fit. Which of the two is most_recent depends on release timing.
  source_ami_filter {
    filters = {
      name                = "al2023-ami-2023*-kernel-6.1-x86_64"
      architecture        = "x86_64"
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

  provisioner "shell" {
    inline = ["mkdir -p /tmp/vault-image"]
  }

  provisioner "file" {
    source      = "vault-utils"
    destination = "/tmp/vault-utils"
  }

  provisioner "file" {
    source      = "files/"
    destination = "/tmp/vault-image"
  }

  provisioner "file" {
    source      = "aws/vault-node-configure"
    destination = "/tmp/vault-image/vault-node-configure"
  }

  provisioner "shell" {
    execute_command = "chmod +x {{ .Path }}; sudo -E sh -c '{{ .Vars }} {{ .Path }}'"
    environment_vars = [
      "VAULT_VERSION=${var.vault_version}",
    ]
    script = "${path.root}/provision.sh"
  }
}
