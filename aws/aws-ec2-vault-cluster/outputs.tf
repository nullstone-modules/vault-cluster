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

output "operator_secret_arn" {
  value       = aws_secretsmanager_secret.platform["operator"].arn
  description = "string ||| Secrets Manager ARN for the operator token."
}

output "provisioning_secret_arn" {
  value       = aws_secretsmanager_secret.platform["provisioning"].arn
  description = "string ||| Secrets Manager ARN for the provisioning token."
}
