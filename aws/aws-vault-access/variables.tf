variable "app_metadata" {
  description = "Injected by Nullstone from the application module. Reserved for capabilities."
  type        = map(string)
  default     = {}
}

variable "vault_role" {
  description = "Vault AWS auth role this app is allowed to use."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9_-]+$", var.vault_role))
    error_message = "vault_role must contain only letters, numbers, hyphens, and underscores."
  }
}

variable "vault_policies" {
  description = "Vault policies granted by vault_role. Platform and tenant policy names are rejected."
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for policy in var.vault_policies :
      can(regex("^[A-Za-z0-9_-]+$", policy)) &&
      !contains(["admin", "aws-auth", "default", "operator", "provisioning", "root"], policy) &&
      !startswith(policy, "tenant-")
    ])
    error_message = "vault_policies cannot name a platform or tenant policy."
  }
}

locals {
  security_group_id = var.app_metadata["security_group_id"]
}
