output "connection_name" {
  description = "Cloud SQL connection name (project:region:instance)."
  value       = google_sql_database_instance.this.connection_name
}

output "private_ip" {
  value = google_sql_database_instance.this.private_ip_address
}

output "db_name" {
  value = google_sql_database.app.name
}
