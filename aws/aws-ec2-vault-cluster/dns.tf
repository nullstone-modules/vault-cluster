resource "aws_route53_record" "vault" {
  zone_id = local.internal_zone_id
  name    = "vault"
  type    = "A"

  alias {
    name                   = aws_lb.this.dns_name
    zone_id                = aws_lb.this.zone_id
    evaluate_target_health = false
  }
}
