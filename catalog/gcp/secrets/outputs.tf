output "secret_ids" {
  description = "Map of logical secret name to Secret Manager secret id."
  value       = { for name, s in google_secret_manager_secret.this : name => s.secret_id }
}
