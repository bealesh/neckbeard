# Bootstrap (DESIGN §10.3), GCP flavor: state bucket first, then workload
# identity federation for CI (no service account keys ever, §10.1), then the
# per-env plan/apply service accounts. This root keeps LOCAL state; the runbook
# treats its state file as an operator artifact.

data "google_project" "this" {}

# --- 1. state backend ---

resource "google_storage_bucket" "tfstate" {
  name                        = "${var.name_prefix}-tfstate"
  location                    = var.region
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }
}

# --- 2. workload identity federation ---

locals {
  github     = var.vcs == "github"
  issuer_uri = local.github ? "https://token.actions.githubusercontent.com" : "https://gitlab.com"
  repo_attr  = local.github ? "assertion.repository" : "assertion.project_path"
  # Pool/provider ids: 4-32 chars, lowercase letters/digits/dashes.
  pool_id = trimsuffix(substr("${var.name_prefix}-ci", 0, 32), "-")
}

resource "google_iam_workload_identity_pool" "ci" {
  workload_identity_pool_id = local.pool_id
  display_name              = "${var.environment} CI"
}

resource "google_iam_workload_identity_pool_provider" "ci" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.ci.workload_identity_pool_id
  workload_identity_pool_provider_id = var.vcs
  display_name                       = var.vcs

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = local.repo_attr
  }

  # Only this repository's workflows may federate — the trust boundary in one line.
  attribute_condition = "attribute.repository == \"${var.repo}\""

  oidc {
    issuer_uri = local.issuer_uri
  }
}

# --- 3. plan (read) and apply (write) service accounts ---

resource "google_service_account" "plan" {
  account_id   = trimsuffix(substr("${var.name_prefix}-ci-plan", 0, 30), "-")
  display_name = "${var.name_prefix} CI plan"
}

resource "google_service_account" "apply" {
  account_id   = trimsuffix(substr("${var.name_prefix}-ci-apply", 0, 30), "-")
  display_name = "${var.name_prefix} CI apply"
}

resource "google_project_iam_member" "plan_viewer" {
  project = data.google_project.this.project_id
  role    = "roles/viewer"
  member  = "serviceAccount:${google_service_account.plan.email}"
}

# Apply needs to create resources AND grant the app-scoped IAM the catalog
# modules declare (per-secret accessors, workload identity bindings). Editor +
# project IAM admin is the honest current grant; least-privilege refinement is
# roadmap work the release harness will shape.
resource "google_project_iam_member" "apply" {
  for_each = toset([
    "roles/editor",
    "roles/resourcemanager.projectIamAdmin",
    "roles/iam.serviceAccountAdmin",
    "roles/container.admin",
  ])
  project = data.google_project.this.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.apply.email}"
}

resource "google_storage_bucket_iam_member" "state_rw" {
  for_each = {
    plan  = google_service_account.plan.email
    apply = google_service_account.apply.email
  }
  bucket = google_storage_bucket.tfstate.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${each.value}"
}

# The federated repository may impersonate both service accounts; which one a job
# uses is workflow logic, gated for prd by environment protection / the protected
# branch (§11.3).
resource "google_service_account_iam_member" "wif" {
  for_each = {
    plan  = google_service_account.plan.name
    apply = google_service_account.apply.name
  }
  service_account_id = each.value
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.ci.name}/attribute.repository/${var.repo}"
}
