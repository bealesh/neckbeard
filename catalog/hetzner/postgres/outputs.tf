output "volume_id" {
  description = "Block-storage volume the runtime mounts for PGDATA."
  value       = hcloud_volume.data.id
}

output "backup_retention_days" {
  value = var.backup_retention_days
}

output "endpoint" {
  description = "Where the app reaches Postgres: the on-box container, by compose service name."
  value       = "postgres:5432"
}
