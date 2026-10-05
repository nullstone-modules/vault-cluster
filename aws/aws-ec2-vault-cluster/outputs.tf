output "ami_id" {
  value       = local.ami
  description = "string ||| AMI ID for Vault nodes."
}

output "role_name" {
  value       = aws_iam_role.this.name
  description = "string ||| IAM role name for Vault EC2 instances."
}

output "instance_profile_name" {
  value       = aws_iam_instance_profile.this.name
  description = "string ||| Instance profile name for the launch template."
}

output "security_group_id" {
  value       = aws_security_group.nodes.id
  description = "string ||| Security group attached to Vault nodes."
}

output "nlb_security_group_id" {
  value       = aws_security_group.nlb.id
  description = "string ||| Security group attached to the internal NLB."
}

output "nlb_dns_name" {
  value       = aws_lb.this.dns_name
  description = "string ||| Internal NLB DNS name for the Vault API."
}

output "vault_fqdn" {
  value       = trimsuffix(aws_route53_record.vault.fqdn, ".")
  description = "string ||| Internal DNS name for the Vault API (vault.internal)."
}

output "admin_function_name" {
  value       = module.vault_admin.function_name
  description = "string ||| In-VPC function that binds an app IAM role to one Vault AWS auth role."
}

output "vault_api_port" {
  value       = tostring(local.vault_api_port)
  description = "string ||| Vault API port."
}

output "user_fqdn" {
  value       = local.user_fqdn
  description = "string ||| User-facing DNS name when a subdomain is connected."
}

output "autoscaling_group_name" {
  value       = aws_autoscaling_group.this.name
  description = "string ||| Auto Scaling Group name for Vault nodes."
}

output "operator_secret_arn" {
  value       = aws_secretsmanager_secret.platform["operator"].arn
  description = "string ||| Secrets Manager ARN for the operator token."
}

output "provisioning_secret_arn" {
  value       = aws_secretsmanager_secret.platform["provisioning"].arn
  description = "string ||| Secrets Manager ARN for the provisioning token."
}

output "vault_addr" {
  value       = local.vault_addr
  description = "string ||| Vault URL on vault.internal, with the scheme the NLB listens on."
}

output "user_vault_addr" {
  value       = local.user_vault_addr
  description = "string ||| Vault URL on the user-facing name, or empty when no subdomain is connected."
}

output "tls_server_name" {
  value       = local.tls_server_name
  description = "string ||| Name to verify in the NLB certificate when connecting to vault_addr, or empty without TLS."
}

output "admin_role_arns" {
  value       = { for k, r in aws_iam_role.admin : k => r.arn }
  description = "map(string) ||| IAM role per admin access level. Group members assume it, then run vault login -method=aws role=admin-<level>."
}

output "admin_group_names" {
  value       = { for k, g in aws_iam_group.admin : k => g.name }
  description = "map(string) ||| IAM group per admin access level. Add IAM users to grant Vault admin access."
}
