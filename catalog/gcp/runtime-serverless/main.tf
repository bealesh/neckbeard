# Serverless-containers runtime on Cloud Run v2. One service per http entry
# (internal ingress, fronted by the dns-ingress load balancer), one worker pool per
# worker (no HTTP listener required — unlike a Cloud Run service), one job +
# Cloud Scheduler trigger per cron. Cloud Run scales to zero; the tier presets'
# min_instances = 0 genuinely costs nothing while idle (DESIGN §3.3).

locals {
  cpu = {
    "0.5" = "500m"
    "1"   = "1"
    "2"   = "2"
  }[var.cpu]
  memory = "${floor(var.memory_gb * 1024)}Mi"

  http_services   = { for s in var.services : s.name => s if s.kind == "http" }
  worker_services = { for s in var.services : s.name => s if s.kind == "worker" }
  cron_services   = { for s in var.services : s.name => s if s.kind == "cron" }
}

data "google_project" "this" {}

resource "google_service_account" "run" {
  # Account ids are 6-30 chars; the prefix is trimmed deterministically.
  account_id   = substr("${var.name_prefix}-run", 0, 30)
  display_name = "${var.name_prefix} runtime"
}

# The runtime identity may read exactly the app's secrets — nothing else
# (least privilege, DESIGN §10.1).
resource "google_secret_manager_secret_iam_member" "run" {
  for_each  = var.secret_ids
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.run.email}"
}

resource "google_cloud_run_v2_service" "http" {
  for_each = local.http_services
  name     = "${var.name_prefix}-${each.key}"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"

  template {
    service_account = google_service_account.run.email

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    vpc_access {
      egress = "PRIVATE_RANGES_ONLY"
      network_interfaces {
        subnetwork = var.subnet_id
      }
    }

    containers {
      image = var.image
      args  = length(each.value.args) > 0 ? each.value.args : null

      ports {
        container_port = each.value.port
      }

      resources {
        limits = {
          cpu    = local.cpu
          memory = local.memory
        }
      }

      dynamic "env" {
        for_each = contains(keys(var.secret_ids), "APP_ENV") ? [] : [1]
        content {
          name  = "APP_ENV"
          value = var.environment
        }
      }
      dynamic "env" {
        for_each = var.secret_ids
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value
              version = "latest"
            }
          }
        }
      }

      startup_probe {
        http_get {
          path = each.value.health_path
          port = each.value.port
        }
      }
    }
  }

  # The release flow updates the image; day-to-day applies must not fight it.
  lifecycle {
    ignore_changes = [template[0].containers[0].image]
  }

  depends_on = [google_secret_manager_secret_iam_member.run]
}

resource "google_cloud_run_v2_worker_pool" "worker" {
  for_each = local.worker_services
  name     = "${var.name_prefix}-${each.key}"
  location = var.region

  template {
    service_account = google_service_account.run.email

    vpc_access {
      egress = "PRIVATE_RANGES_ONLY"
      network_interfaces {
        subnetwork = var.subnet_id
      }
    }

    containers {
      image = var.image
      args  = length(each.value.args) > 0 ? each.value.args : null

      resources {
        limits = {
          cpu    = local.cpu
          memory = local.memory
        }
      }

      dynamic "env" {
        for_each = contains(keys(var.secret_ids), "APP_ENV") ? [] : [1]
        content {
          name  = "APP_ENV"
          value = var.environment
        }
      }
      dynamic "env" {
        for_each = var.secret_ids
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value
              version = "latest"
            }
          }
        }
      }
    }
  }

  scaling {
    manual_instance_count = var.min_instances > 0 ? var.min_instances : 1
  }

  lifecycle {
    ignore_changes = [template[0].containers[0].image]
  }

  depends_on = [google_secret_manager_secret_iam_member.run]
}

resource "google_cloud_run_v2_job" "cron" {
  for_each = local.cron_services
  name     = "${var.name_prefix}-${each.key}"
  location = var.region

  template {
    template {
      service_account = google_service_account.run.email

      vpc_access {
        egress = "PRIVATE_RANGES_ONLY"
        network_interfaces {
          subnetwork = var.subnet_id
        }
      }

      containers {
        image = var.image
        args  = length(each.value.args) > 0 ? each.value.args : null

        resources {
          limits = {
            cpu    = local.cpu
            memory = local.memory
          }
        }

        dynamic "env" {
          for_each = contains(keys(var.secret_ids), "APP_ENV") ? [] : [1]
          content {
            name  = "APP_ENV"
            value = var.environment
          }
        }
        dynamic "env" {
          for_each = var.secret_ids
          content {
            name = env.key
            value_source {
              secret_key_ref {
                secret  = env.value
                version = "latest"
              }
            }
          }
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [template[0].template[0].containers[0].image]
  }

  depends_on = [google_secret_manager_secret_iam_member.run]
}

# Scheduler triggers the job over the Run Admin API with the runtime identity,
# which needs run.invoker on the job. Cloud Scheduler accepts 5-field cron as-is —
# no translation, an honest difference from EventBridge.
resource "google_cloud_run_v2_job_iam_member" "scheduler_invokes" {
  for_each = local.cron_services
  name     = google_cloud_run_v2_job.cron[each.key].name
  location = var.region
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.run.email}"
}

resource "google_cloud_scheduler_job" "cron" {
  for_each = local.cron_services
  name     = "${var.name_prefix}-${each.key}"
  region   = var.region
  schedule = each.value.schedule

  http_target {
    http_method = "POST"
    uri         = "https://run.googleapis.com/v2/projects/${data.google_project.this.project_id}/locations/${var.region}/jobs/${google_cloud_run_v2_job.cron[each.key].name}:run"

    oauth_token {
      service_account_email = google_service_account.run.email
    }
  }
}

# The load balancer does not supply a Cloud Run identity token. IAM permits
# invocation while ingress remains restricted to internal/load-balancer traffic.
resource "google_cloud_run_v2_service_iam_member" "ingress" {
  for_each = local.http_services
  name     = google_cloud_run_v2_service.http[each.key].name
  location = var.region
  role     = "roles/run.invoker"
  member   = "allUsers"
}
