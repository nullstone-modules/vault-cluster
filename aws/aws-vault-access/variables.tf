variable "app_metadata" {
  description = "Injected by Nullstone from the application module. Reserved for capabilities."
  type        = map(string)
  default     = {}
}

variable "role_name" {
  description = "Vault AWS auth role this app is allowed to use. If empty, the role is <app-name>-<resource-suffix>."
  type        = string
  default     = ""

  validation {
    condition     = var.role_name == "" || can(regex("^[A-Za-z0-9_-]+$", var.role_name))
    error_message = "role_name must contain only letters, numbers, hyphens, and underscores."
  }
}

variable "policies" {
  description = "Vault policies granted by role_name. Platform and tenant policy names are rejected."
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for policy in var.policies :
      can(regex("^[A-Za-z0-9_-]+$", policy)) &&
      !contains(["admin", "aws-auth", "default", "operator", "provisioning", "root"], policy) &&
      !startswith(policy, "tenant-")
    ])
    error_message = "policies cannot name a platform or tenant policy."
  }
}

locals {
  security_group_id = var.app_metadata["security_group_id"]
}
