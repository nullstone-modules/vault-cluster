resource "aws_autoscaling_group" "this" {
  name_prefix         = "${local.resource_name}-"
  vpc_zone_identifier = local.private_subnet_ids
  desired_capacity    = var.cluster_size
  min_size            = var.cluster_size
  max_size            = var.cluster_size
  target_group_arns   = [aws_lb_target_group.api.arn]

  health_check_type         = "EC2"
  health_check_grace_period = 600

  launch_template {
    id      = aws_launch_template.this.id
    version = "$Latest"
  }

  tag {
    key                 = "Name"
    value               = local.resource_name
    propagate_at_launch = true
  }

  tag {
    key                 = local.vault_cluster_tag_key
    value               = local.vault_cluster_tag_value
    propagate_at_launch = true
  }
}
