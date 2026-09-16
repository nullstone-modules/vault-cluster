resource "aws_launch_template" "this" {
  name_prefix   = "${local.resource_name}-"
  image_id      = local.ami
  instance_type = var.instance_type
  user_data     = base64encode(local.user_data)

  iam_instance_profile {
    name = aws_iam_instance_profile.this.name
  }

  vpc_security_group_ids = [aws_security_group.nodes.id]

  metadata_options {
    http_endpoint               = "enabled"
    http_put_response_hop_limit = 1
    http_tokens                 = "required"
  }

  tag_specifications {
    resource_type = "instance"
    tags = merge(local.tags, {
      Name                          = local.resource_name
      (local.vault_cluster_tag_key) = local.vault_cluster_tag_value
    })
  }

  lifecycle {
    create_before_destroy = true
  }
}
