output "state_bucket" {
  value = minio_s3_bucket.tfstate.bucket
}

output "release_store" {
  description = "Versioned storage for verified deployment and rollback receipts (release automation for this lane is not implemented yet)."
  value       = "s3://${minio_s3_bucket.tfstate.bucket}"
}

output "backend_hcl" {
  description = "Contents for infra/envs/<env>/backend.hcl. The s3 backend talks to Object Storage; credentials come from AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY in the environment."
  value       = <<-EOT
    bucket                      = "${minio_s3_bucket.tfstate.bucket}"
    key                         = "envs/${var.environment}/terraform.tfstate"
    region                      = "${var.region}"
    endpoints                   = { s3 = "https://${local.endpoint}" }
    use_path_style              = true
    skip_credentials_validation = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true
    use_lockfile                = true
  EOT
}

output "ci_variables" {
  description = "Non-secret CI variables the generated pipelines expect. The three secrets (NECKBEARD_HCLOUD_TOKEN_<ENV>, NECKBEARD_HETZNER_S3_KEY_<ENV>, NECKBEARD_HETZNER_S3_SECRET_<ENV>) are set by hand; see docs/bootstrap.md."
  value = {
    "NECKBEARD_HETZNER_READY_${upper(var.environment)}" = "true"
    "NECKBEARD_REGISTRY_${upper(var.environment)}"      = "${local.registry_host}/${lower(var.repo)}"
  }
}
