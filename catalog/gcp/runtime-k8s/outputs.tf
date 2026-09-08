output "cluster_name" {
  value = google_container_cluster.this.name
}

output "cluster_endpoint" {
  value = google_container_cluster.this.endpoint
}

output "workload_pool" {
  value = "${data.google_project.this.project_id}.svc.id.goog"
}

output "external_secrets_gsa" {
  description = "GSA email the delivery layer annotates onto the external-secrets KSA."
  value       = google_service_account.external_secrets.email
}
