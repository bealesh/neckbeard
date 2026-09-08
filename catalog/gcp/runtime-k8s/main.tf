# Kubernetes runtime on GKE: regional VPC-native cluster, private nodes, Workload
# Identity, dedicated least-privilege node service account. Delivery (Flux, GKE's
# built-in ingress, external-secrets) is rendered by the clusters layer.
#
# Honest notes (DESIGN §3.3, §10):
# - The API endpoint is public at M2 (private nodes regardless); origin lockdown
#   is a hardening roadmap item, same posture as the EKS lane.
# - A regional control plane bills while idle in every environment (one zonal
#   cluster is cheaper but single-point — availability is a tier dimension).

data "google_project" "this" {}

locals {
  machine_type = {
    small  = "e2-medium"
    medium = "e2-standard-4"
  }[var.node_shape]
}

resource "google_service_account" "nodes" {
  account_id   = substr("${var.name_prefix}-nodes", 0, 30)
  display_name = "${var.name_prefix} GKE nodes"
}

resource "google_project_iam_member" "nodes" {
  for_each = toset([
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter",
    "roles/monitoring.viewer",
    "roles/artifactregistry.reader",
  ])
  project = data.google_project.this.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.nodes.email}"
}

resource "google_container_cluster" "this" {
  name     = var.name_prefix
  location = var.region

  network    = var.network_id
  subnetwork = var.subnet_id

  remove_default_node_pool = true
  initial_node_count       = 1

  release_channel {
    channel = "REGULAR"
  }

  # Dataplane V2: eBPF dataplane with native NetworkPolicy enforcement.
  datapath_provider = "ADVANCED_DATAPATH"

  master_auth {
    client_certificate_config {
      issue_client_certificate = false
    }
  }

  resource_labels = {
    managed-by       = "neckbeard"
    neckbeard-prefix = var.name_prefix
  }

  workload_identity_config {
    workload_pool = "${data.google_project.this.project_id}.svc.id.goog"
  }

  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
  }

  # VPC-native (alias IP) — required for private nodes and container-native LB.
  ip_allocation_policy {}

  # Teardown-ability for the release matrix; a prd preset input can harden this
  # later, mirroring the database's deletion_protection.
  deletion_protection = false
}

resource "google_container_node_pool" "default" {
  name     = "default"
  cluster  = google_container_cluster.this.id
  location = var.region

  autoscaling {
    total_min_node_count = var.min_nodes
    total_max_node_count = var.max_nodes
  }

  node_config {
    machine_type    = local.machine_type
    spot            = var.spot
    service_account = google_service_account.nodes.email
    oauth_scopes    = ["https://www.googleapis.com/auth/cloud-platform"]

    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }
}

# --- external-secrets identity (Workload Identity) ---
# The delivery layer annotates the external-secrets KSA with this GSA; access is
# scoped to exactly the app's secrets (least privilege, §10.1).

resource "google_service_account" "external_secrets" {
  account_id   = substr("${var.name_prefix}-eso", 0, 30)
  display_name = "${var.name_prefix} external-secrets"
}

resource "google_secret_manager_secret_iam_member" "external_secrets" {
  for_each  = var.secret_ids
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.external_secrets.email}"
}

resource "google_service_account_iam_member" "external_secrets_wi" {
  service_account_id = google_service_account.external_secrets.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${data.google_project.this.project_id}.svc.id.goog[external-secrets/external-secrets]"
}
