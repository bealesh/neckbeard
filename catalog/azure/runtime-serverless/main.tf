# Serverless-containers runtime on Azure Container Apps. One app per service; http
# services use the platform's built-in HTTPS ingress (no dns-ingress module on this
# lane — honest per-cloud difference, DESIGN §3.3), workers run without ingress,
# cron services run as Container App Jobs. Container Apps scales to zero and its
# scaling is KEDA-based (§3.3).

locals {
  # Container Apps consumption workloads accept fixed cpu/memory pairs; the catalog
  # sizes map onto them exactly (0.5→1Gi, 1→2Gi, 2→4Gi), enforced by the
  # precondition on the environment below.
  cpu_cores = {
    "0.5" = 0.5
    "1"   = 1
    "2"   = 2
  }[var.cpu]
  memory = "${var.memory_gb}Gi"

  http_services   = { for s in var.services : s.name => s if s.kind == "http" }
  worker_services = { for s in var.services : s.name => s if s.kind == "worker" }
  cron_services   = { for s in var.services : s.name => s if s.kind == "cron" }
  app_services    = merge(local.http_services, local.worker_services)

  # Container App (and Job) names: lowercase alnum + dashes, max 32 chars. Long
  # prefix+service combinations are truncated (and a trailing dash trimmed) —
  # collisions are possible if two service names only differ past the cut, which
  # the workload contract's short service names make unlikely.
  app_name = { for s in var.services :
    s.name => trimsuffix(substr(lower("${var.name_prefix}-${s.name}"), 0, 32), "-")
  }

  # Container Apps secret names must be lowercase alnum/dash; blueprint secret
  # names like DATABASE_URL keep their spelling as env var names and are sanitized
  # only where the platform demands it.
  ca_secret_name = { for name, uri in var.secret_uris : name => lower(replace(name, "_", "-")) }

  cron_fields = { for name, s in local.cron_services : name => split(" ", s.schedule) }
}

resource "azurerm_log_analytics_workspace" "this" {
  name                = "${var.name_prefix}-logs"
  location            = var.region
  resource_group_name = var.resource_group_name
  sku                 = "PerGB2018"
  retention_in_days   = 30
}

# Workload-profiles environment (Consumption profile only at M1): required so the
# environment can live in the delegated /23 subnet from the network module and get
# private, VNet-routed east-west traffic (§10.1).
resource "azurerm_container_app_environment" "this" {
  name                       = "${var.name_prefix}-env"
  location                   = var.region
  resource_group_name        = var.resource_group_name
  log_analytics_workspace_id = azurerm_log_analytics_workspace.this.id
  infrastructure_subnet_id   = var.subnet_id

  workload_profile {
    name                  = "Consumption"
    workload_profile_type = "Consumption"
  }

  lifecycle {
    precondition {
      condition = (
        (local.cpu_cores == 0.5 && var.memory_gb == 1) ||
        (local.cpu_cores == 1 && var.memory_gb == 2) ||
        (local.cpu_cores == 2 && var.memory_gb == 4)
      )
      error_message = "cpu/memory is not a valid Container Apps consumption combination (0.5/1, 1/2, 2/4)."
    }
  }
}

# http and worker services. Secret wiring is by Key Vault REFERENCE only: the app's
# system-assigned identity reads the versionless secret URI at runtime — no secret
# value ever passes through neckbeard or OpenTofu state (§10.1). Honest ordering
# caveat: Container Apps resolves each referenced secret when the app is created,
# so the FIRST apply of an environment fails until operators have set the secret
# values out-of-band (the runbook makes setting them a pre-apply step). Static
# validation and planning are unaffected.

# ACR pulls use a dedicated user-assigned identity with AcrPull, created before
# any revision references a private image — a system identity cannot, because its
# role assignment can only follow app creation while the first private-image
# revision needs pull rights immediately (V2 finding, 2026-09-08).
resource "azurerm_user_assigned_identity" "acr_pull" {
  name                = "${var.name_prefix}-acr-pull"
  location            = var.region
  resource_group_name = var.resource_group_name
}

resource "azurerm_role_assignment" "acr_pull" {
  scope                = var.registry_id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.acr_pull.principal_id
}

resource "azurerm_container_app" "this" {
  for_each                     = local.app_services
  name                         = local.app_name[each.key]
  container_app_environment_id = azurerm_container_app_environment.this.id
  resource_group_name          = var.resource_group_name
  revision_mode                = "Single"
  workload_profile_name        = "Consumption"

  identity {
    type         = "SystemAssigned, UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.acr_pull.id]
  }

  registry {
    server   = var.registry_server
    identity = azurerm_user_assigned_identity.acr_pull.id
  }

  dynamic "secret" {
    for_each = var.secret_uris
    content {
      name                = local.ca_secret_name[secret.key]
      identity            = "System"
      key_vault_secret_id = secret.value
    }
  }

  # http ingress comes from the platform (built-in HTTPS on *.azurecontainerapps.io
  # at M1); workers get no ingress at all.
  dynamic "ingress" {
    for_each = each.value.kind == "http" ? [each.value] : []
    content {
      external_enabled = true
      target_port      = ingress.value.port
      transport        = "auto"

      traffic_weight {
        latest_revision = true
        percentage      = 100
      }
    }
  }

  template {
    # Workers keep a floor of one replica: without ingress, Container Apps' default
    # (HTTP) scaler never wakes a scaled-to-zero app, and the workload contract's
    # workers are queue-less poll loops with no KEDA-visible signal to scale on
    # (§3.1). Honest difference from http services, which do scale to zero.
    min_replicas = each.value.kind == "worker" ? max(var.min_instances, 1) : var.min_instances
    max_replicas = var.max_instances

    container {
      name   = each.key
      image  = var.image
      cpu    = local.cpu_cores
      memory = local.memory
      args   = each.value.args

      dynamic "env" {
        for_each = var.secret_uris
        content {
          name        = env.key
          secret_name = local.ca_secret_name[env.key]
        }
      }

      dynamic "liveness_probe" {
        for_each = each.value.kind == "http" ? [each.value] : []
        content {
          transport = "HTTP"
          port      = liveness_probe.value.port
          path      = liveness_probe.value.health_path
        }
      }
    }
  }

  lifecycle {
    # The release flow owns the image (digest bumps via the per-env release state,
    # DESIGN §11.2, D3); day-to-day infra applies must not fight it.
    ignore_changes = [template[0].container[0].image]
  }
}

# Cron services: Container App Jobs. Azure accepts standard 5-field cron
# expressions directly — no translation layer, unlike the AWS lane (§3.3).
resource "azurerm_container_app_job" "cron" {
  for_each                     = local.cron_services
  name                         = local.app_name[each.key]
  location                     = var.region
  container_app_environment_id = azurerm_container_app_environment.this.id
  resource_group_name          = var.resource_group_name
  workload_profile_name        = "Consumption"

  replica_timeout_in_seconds = 1800
  replica_retry_limit        = 1

  schedule_trigger_config {
    cron_expression          = each.value.schedule
    parallelism              = 1
    replica_completion_count = 1
  }

  identity {
    type         = "SystemAssigned, UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.acr_pull.id]
  }

  registry {
    server   = var.registry_server
    identity = azurerm_user_assigned_identity.acr_pull.id
  }

  dynamic "secret" {
    for_each = var.secret_uris
    content {
      name                = local.ca_secret_name[secret.key]
      identity            = "System"
      key_vault_secret_id = secret.value
    }
  }

  template {
    container {
      name   = each.key
      image  = var.image
      cpu    = local.cpu_cores
      memory = local.memory
      args   = each.value.args

      dynamic "env" {
        for_each = var.secret_uris
        content {
          name        = env.key
          secret_name = local.ca_secret_name[env.key]
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [template[0].container[0].image]

    precondition {
      condition     = length(local.cron_fields[each.key]) == 5
      error_message = "cron schedules must be standard 5-field expressions."
    }
  }
}

# Each app/job reads its Key Vault references with its own system-assigned
# identity: "Key Vault Secrets User" scoped to the single vault. Gated on
# secret_uris rather than key_vault_id because the map's KEYS are known at plan
# time (they come from declared secret names) while key_vault_id is an apply-time
# module output — a for_each on it would fail `tofu plan` on a fresh environment.
resource "azurerm_role_assignment" "app_kv" {
  for_each             = length(var.secret_uris) > 0 ? local.app_services : {}
  scope                = var.key_vault_id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_container_app.this[each.key].identity[0].principal_id
}

resource "azurerm_role_assignment" "job_kv" {
  for_each             = length(var.secret_uris) > 0 ? local.cron_services : {}
  scope                = var.key_vault_id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_container_app_job.cron[each.key].identity[0].principal_id
}
