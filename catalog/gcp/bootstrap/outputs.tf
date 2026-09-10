output "state_bucket" {
  value = google_storage_bucket.tfstate.name
}

output "release_store" {
  description = "Versioned storage for verified deployment and rollback receipts."
  value       = "gs://${google_storage_bucket.tfstate.name}"
}

output "backend_hcl" {
  description = "Contents for infra/envs/<env>/backend.hcl (the pipelines init with it)."
  value       = <<-EOT
    bucket = "${google_storage_bucket.tfstate.name}"
    prefix = "envs/${var.environment}"
  EOT
}

output "workload_identity_provider" {
  description = "Full provider resource name for the CI federation step."
  value       = google_iam_workload_identity_pool_provider.ci.name
}

output "ci_variables" {
  description = "CI variables the generated pipelines expect (set via gh/glab, see docs/bootstrap.md)."
  value = {
    "NECKBEARD_GCP_WIF_PROVIDER_${upper(var.environment)}" = google_iam_workload_identity_pool_provider.ci.name
    "NECKBEARD_GCP_PLAN_SA_${upper(var.environment)}"      = google_service_account.plan.email
    "NECKBEARD_GCP_APPLY_SA_${upper(var.environment)}"     = google_service_account.apply.email
  }
}
