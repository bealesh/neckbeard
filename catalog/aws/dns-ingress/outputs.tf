output "alb_dns_name" {
  value = aws_lb.this.dns_name
}

output "alb_security_group_id" {
  value = aws_security_group.alb.id
}

output "target_group_arns" {
  description = "Map of http service name to target group ARN."
  value       = { for name, tg in aws_lb_target_group.service : name => tg.arn }
}
