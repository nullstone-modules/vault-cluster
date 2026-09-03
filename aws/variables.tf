variable "cluster_size" {
  type        = number
  default     = 1
  description = "Vault nodes. 1 is non-prod (no quorum). 3 is prod/staging (one per AZ)."

  validation {
    condition     = contains([1, 3], var.cluster_size)
    error_message = "cluster_size must be 1 or 3."
  }
}

variable "instance_type" {
  type        = string
  default     = "t4g.micro"
  description = "EC2 type. Default is the cheap non-prod size."
}

variable "backup_schedule" {
  type        = string
  default     = "off"
  description = "Raft snapshot cadence to the connected S3 bucket: hourly, daily, or off."

  validation {
    condition     = contains(["hourly", "daily", "off"], var.backup_schedule)
    error_message = "backup_schedule must be hourly, daily, or off."
  }
}
