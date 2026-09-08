output "endpoint" {
  value = aws_db_instance.this.endpoint
}

output "db_name" {
  value = aws_db_instance.this.db_name
}

output "master_user_secret_arn" {
  description = "Secrets Manager ARN of the RDS-managed master credentials."
  value       = one(aws_db_instance.this.master_user_secret[*].secret_arn)
}

output "replica_endpoints" {
  value = aws_db_instance.replica[*].endpoint
}
