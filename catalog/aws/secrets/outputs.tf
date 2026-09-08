output "secret_arns" {
  description = "Map of secret name to ARN."
  value       = { for name, s in aws_secretsmanager_secret.this : name => s.arn }
}
