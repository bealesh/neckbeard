# Managed PostgreSQL (Cloud SQL) on private IP only. No database credential is
# created here: the app's DATABASE_URL secret is operator-provided (DESIGN §8) —
# unlike RDS there is no provider-managed master password that stays out of state,
# an honest per-cloud difference (§3.3).

locals {
  # Shared-core tiers exist for Postgres and fit the smallest presets; medium is a
  # dedicated-core custom shape (2 vCPU / 8GB).
  sql_tier = {
    smallest = "db-f1-micro"
    small    = "db-g1-small"
    medium   = "db-custom-2-8192"
  }[var.instance_class]
}

resource "google_sql_database_instance" "this" {
  name                = "${var.name_prefix}-pg"
  database_version    = "POSTGRES_17"
  region              = var.region
  deletion_protection = var.deletion_protection

  settings {
    tier              = local.sql_tier
    availability_type = var.multi_zone ? "REGIONAL" : "ZONAL"
    disk_autoresize   = true

    ip_configuration {
      ipv4_enabled    = false
      private_network = var.network_id
      ssl_mode        = "ENCRYPTED_ONLY"
    }

    database_flags {
      name  = "log_checkpoints"
      value = "on"
    }
    database_flags {
      name  = "log_connections"
      value = "on"
    }
    database_flags {
      name  = "log_disconnections"
      value = "on"
    }
    database_flags {
      name  = "log_lock_waits"
      value = "on"
    }

    backup_configuration {
      enabled                        = var.backup_retention_days > 0
      point_in_time_recovery_enabled = var.pitr
      backup_retention_settings {
        retained_backups = var.backup_retention_days
      }
    }
  }

  lifecycle {
    precondition {
      condition     = var.private_services_connection != ""
      error_message = "The network module's private services access connection must exist before Cloud SQL can get a private IP."
    }
    precondition {
      condition     = !var.pitr || var.backup_retention_days > 0
      error_message = "pitr requires backups (backup_retention_days > 0)."
    }
  }
}

resource "google_sql_database" "app" {
  name     = "app"
  instance = google_sql_database_instance.this.name
}

resource "google_sql_database_instance" "replica" {
  count                = var.read_replicas
  name                 = "${var.name_prefix}-pg-ro-${count.index}"
  master_instance_name = google_sql_database_instance.this.name
  database_version     = "POSTGRES_17"
  region               = var.region
  deletion_protection  = false

  settings {
    tier            = local.sql_tier
    disk_autoresize = true

    ip_configuration {
      ipv4_enabled    = false
      private_network = var.network_id
      ssl_mode        = "ENCRYPTED_ONLY"
    }

    database_flags {
      name  = "log_checkpoints"
      value = "on"
    }
    database_flags {
      name  = "log_connections"
      value = "on"
    }
    database_flags {
      name  = "log_disconnections"
      value = "on"
    }
    database_flags {
      name  = "log_lock_waits"
      value = "on"
    }
  }
}
