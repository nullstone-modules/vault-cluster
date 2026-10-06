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

variable "access" {
  description = "Tenant access level: reader (KV read) or writer (KV read/write and database credentials). The app logs in as one tenant at a time at this level."
  type        = string
  default     = "reader"

  validation {
    condition     = contains(["reader", "writer"], var.access)
    error_message = "access must be reader or writer."
  }
}

locals {
  security_group_id = var.app_metadata["security_group_id"]
  # Cluster convention: one AppRole mount per level, role name = tenant ID (vault-utils AUTH_MOUNT, default approle).
  tenant_mount = "approle-${var.access}"
}
