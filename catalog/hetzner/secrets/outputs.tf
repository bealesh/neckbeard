output "secret_names" {
  value = sort(var.secret_names)
}

output "env_file_template" {
  description = "NAME= lines, one per secret, for /etc/neckbeard/app.env on the server."
  value       = local.env_template
}

output "env_file_path" {
  value = "/etc/neckbeard/app.env"
}
