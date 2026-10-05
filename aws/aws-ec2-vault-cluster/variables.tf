variable "cluster_size" {
  type        = number
  default     = 1
  description = <<EOF
The number of Vault nodes to run.
For non-production environments, set to 1 to run a cheaper cluster.
For production environments, choose an odd number of instances (3, 5, 7, etc.) to add high-availability.
EOF

  validation {
    condition     = var.cluster_size >= 1 && var.cluster_size % 2 == 1
    error_message = "cluster_size must be an odd number of instances (1, 3, 5, 7, ...)."
  }
}

variable "instance_type" {
  type        = string
  default     = "t3.micro"
  description = <<EOF
Instance Type that dictates CPU, Memory, network bandwidth, and file storage type and bandwidth.
See https://aws.amazon.com/ec2/instance-types/ for EC2 instance types.
EOF
}

variable "ami" {
  type        = string
  default     = ""
  description = <<EOF
AMI ID for Vault nodes. Leave empty to use the latest account AMI tagged `Name=nullstone-vault` (x86_64, baked by vault-node/vault.pkr.hcl).
The image must contain Vault CE, vault-utils, and amazon-ssm-agent.
EOF
}

variable "backup_schedule" {
  type        = string
  default     = ""
  description = <<EOF
Cron expression for Raft snapshots to the connected S3 bucket.
Leave empty to disable scheduled snapshots.
Use a schedule that avoids this environment's peak traffic.
EOF
}

variable "protect_platform_secrets" {
  type        = bool
  default     = true
  description = <<EOF
Prevent destroy of the init, provisioning, and operator Secrets Manager secrets.
Leave enabled for normal use. Set to false before destroying this workspace so the stack can be deleted.
EOF
}

variable "admin_require_mfa" {
  type        = bool
  default     = true
  description = <<EOF
Require MFA when an admin group member assumes a Vault admin role.
EOF
}

variable "admin_principals" {
  type = map(object({
    principal_arn = string
    access        = list(string)
  }))
  default     = {}
  description = <<EOF
Extra IAM principals bound to Vault admin roles, for principals that cannot join the admin IAM groups
(IAM Identity Center permission sets, roles in other accounts). Each key becomes the Vault role `admin-<key>`.
`principal_arn` is an IAM user or role ARN; a trailing `*` matches a prefix, e.g.
`arn:aws:iam::123456789012:role/aws-reserved/sso.amazonaws.com/AWSReservedSSO_VaultAdmin_*`.
`access` is any of `tenants` (onboard and offboard tenants) and `operator` (health, snapshots, generate-root).
EOF

  validation {
    condition     = alltrue([for k in keys(var.admin_principals) : can(regex("^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$", k)) && !contains(["tenants", "operator"], k)])
    error_message = "admin_principals keys must be 1-32 lowercase letters, digits, or hyphens, and cannot be tenants or operator."
  }

  validation {
    condition     = alltrue([for v in values(var.admin_principals) : can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:(user|role)/[A-Za-z0-9+=,.@_/-]+[*]?$", v.principal_arn))])
    error_message = "admin_principals principal_arn must be an IAM user or role ARN, optionally ending in *."
  }

  validation {
    condition     = alltrue([for v in values(var.admin_principals) : length(v.access) > 0 && alltrue([for a in v.access : contains(["tenants", "operator"], a)])])
    error_message = "admin_principals access must list one or more of: tenants, operator."
  }

  validation {
    condition     = length(distinct([for v in values(var.admin_principals) : v.principal_arn])) == length(var.admin_principals)
    error_message = "admin_principals cannot bind the same principal_arn twice."
  }
}
