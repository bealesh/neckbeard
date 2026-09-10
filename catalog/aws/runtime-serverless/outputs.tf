output "cluster_name" {
  value = aws_ecs_cluster.this.name
}

output "service_security_group_id" {
  description = "Security group of the Fargate tasks; the postgres module allows it on 5432."
  value       = aws_security_group.service.id
}

output "task_role_arn" {
  description = "The app's runtime identity; app-scoped grants attach at the env root."
  value       = aws_iam_role.task.arn
}

output "task_role_name" {
  value = aws_iam_role.task.name
}

output "task_families" {
  value = { for name, task in aws_ecs_task_definition.service : name => task.family }
}
output "cron_rules" {
  value = { for name, rule in aws_cloudwatch_event_rule.cron : name => rule.name }
}
