output "service_names" {
  description = "Map of http service logical name to Cloud Run service name (consumed by dns-ingress serverless NEGs)."
  value       = { for name, s in google_cloud_run_v2_service.http : name => s.name }
}

output "service_urls" {
  description = "Direct Cloud Run URLs (pre-LB; internal-LB ingress means these reject direct public traffic)."
  value       = { for name, s in google_cloud_run_v2_service.http : name => s.uri }
}

output "runtime_service_account" {
  value = google_service_account.run.email
}
