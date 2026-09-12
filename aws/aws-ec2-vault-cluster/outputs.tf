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
