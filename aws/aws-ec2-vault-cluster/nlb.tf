resource "aws_lb" "this" {
  name               = local.resource_name
  internal           = true
  load_balancer_type = "network"
  subnets            = local.private_subnet_ids
  security_groups    = [aws_security_group.nlb.id]
  tags               = local.tags

  enable_cross_zone_load_balancing = true
}

resource "aws_lb_target_group" "api" {
  name        = "${local.resource_name}-api"
  port        = local.vault_api_port
  protocol    = "TCP"
  vpc_id      = local.vpc_id
  target_type = "instance"
  tags        = local.tags

  health_check {
    enabled             = true
    protocol            = "HTTP"
    port                = tostring(local.vault_health_port)
    path                = "/"
    matcher             = "200"
    interval            = 10
    timeout             = 6
    healthy_threshold   = 2
    unhealthy_threshold = 2
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_lb_listener" "api" {
  load_balancer_arn = aws_lb.this.arn
  port              = local.vault_api_port
  protocol          = "TLS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-Res-2021-06"
  certificate_arn   = module.cert.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}
