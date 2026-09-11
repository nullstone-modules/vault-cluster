locals {
  asg_max_healthy_percentage = ceil((var.cluster_size + 1) / var.cluster_size * 100)
}

resource "aws_autoscaling_group" "this" {
  name_prefix         = "${local.resource_name}-"
  vpc_zone_identifier = local.private_subnet_ids
  desired_capacity    = var.cluster_size
  min_size            = var.cluster_size
  max_size            = var.cluster_size + 1
  target_group_arns   = [aws_lb_target_group.api.arn]

  health_check_type         = "EC2"
  health_check_grace_period = 600

  launch_template {
    id      = aws_launch_template.this.id
    version = aws_launch_template.this.latest_version
  }

  instance_refresh {
    strategy = "Rolling"

    preferences {
      min_healthy_percentage = 100
      max_healthy_percentage = local.asg_max_healthy_percentage
      auto_rollback          = true
      skip_matching          = true
    }
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
