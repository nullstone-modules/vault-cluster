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
  default     = "t4g.micro"
  description = <<EOF
Instance Type that dictates CPU, Memory, network bandwidth, and file storage type and bandwidth.
See https://aws.amazon.com/ec2/instance-types/ for EC2 instance types.
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
