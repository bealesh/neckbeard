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

output "deployment_service_names" {
  value = merge(
    { for name, service in google_cloud_run_v2_service.http : name => service.name },
    { for name, worker in google_cloud_run_v2_worker_pool.worker : name => worker.name },
    { for name, job in google_cloud_run_v2_job.cron : name => job.name }
  )
}
