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
  description = "Vault policies granted by vault_role. The app IAM role cannot use any other role."
  type        = list(string)
  default     = []
}

locals {
  security_group_id = var.app_metadata["security_group_id"]
}
