output "network_id" {
  value = google_compute_network.this.id
}

output "subnet_id" {
  value = google_compute_subnetwork.this.id
}

output "private_services_connection" {
  description = "Passed into the postgres module to order Cloud SQL after the peering exists."
  value       = google_service_networking_connection.psa.id
}
