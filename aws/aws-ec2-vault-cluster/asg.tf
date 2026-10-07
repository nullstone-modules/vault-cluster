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
      # Health after the launch hook continues; the hook, not warmup, is what proves the node joined.
      instance_warmup        = 60
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

# A new instance waits here until vault-utils lifecycle reports it an unsealed, caught-up Raft voter. One that
# never joins is abandoned, which fails the instance refresh and rolls it back with the old node untouched.
resource "aws_autoscaling_lifecycle_hook" "join" {
  name                   = "vault-join"
  autoscaling_group_name = aws_autoscaling_group.this.name
  lifecycle_transition   = "autoscaling:EC2_INSTANCE_LAUNCHING"
  default_result         = "ABANDON"
  heartbeat_timeout      = 1800
}

# A departing instance waits here until it has left the Raft peer set, so the remaining nodes keep quorum.
# It terminates either way; vault-utils logs the attempt.
resource "aws_autoscaling_lifecycle_hook" "leave" {
  name                   = "vault-leave"
  autoscaling_group_name = aws_autoscaling_group.this.name
  lifecycle_transition   = "autoscaling:EC2_INSTANCE_TERMINATING"
  default_result         = "CONTINUE"
  heartbeat_timeout      = 300
}
