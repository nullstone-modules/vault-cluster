variable "app_metadata" {
  description = "Injected by Nullstone from the application module. Reserved for capabilities."
  type        = map(string)
  default     = {}
}

locals {
  security_group_id = var.app_metadata["security_group_id"]
}
