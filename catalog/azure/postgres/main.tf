# Managed PostgreSQL (Azure Database for PostgreSQL – Flexible Server), private
# access only: VNet-injected into the delegated subnet, FQDN resolvable only via
# the private DNS zone (DESIGN §10.1). Direct module consumers default to Entra
# authentication. Generated roots opt into password authentication through an
# ephemeral, write-only input; passwords never enter plans, state or Git.

locals {
  # Honest per-cloud difference (§3.3): zone-redundant HA requires a
  # GeneralPurpose or MemoryOptimized SKU — Burstable cannot do it. When
  # multi_zone is set, smallest/small are forced up to the GP SKU; the plan output
  # and cost report carry the real SKU, never the pretended one.
  sku_by_class = {
    smallest = "B_Standard_B1ms"
    small    = "B_Standard_B2s"
    medium   = "GP_Standard_D2ds_v4"
  }
  sku_name = var.multi_zone ? "GP_Standard_D2ds_v4" : local.sku_by_class[var.instance_class]

  # Azure's floor for automated backup retention is 7 days; presets asking for
  # less (dev: 3) are clamped up rather than refused — more retention, not less.
  retention_days = max(var.backup_retention_days, 7)
}

resource "azurerm_postgresql_flexible_server" "this" {
  name                = "${var.name_prefix}-pg"
  location            = var.region
  resource_group_name = var.resource_group_name

  administrator_login               = var.manage_app_credentials ? "neckbeard" : null
  administrator_password_wo         = var.manage_app_credentials ? var.application_password : null
  administrator_password_wo_version = var.manage_app_credentials ? 1 : null
  version                           = "16"
  sku_name                          = local.sku_name
  storage_mb                        = 32768

  delegated_subnet_id           = var.delegated_subnet_id
  private_dns_zone_id           = var.private_dns_zone_id
  public_network_access_enabled = false

  backup_retention_days = local.retention_days

  # Existing direct consumers keep Entra authentication. The managed connection
  # path uses a cloud-stored password supplied only at deployment time.
  authentication {
    active_directory_auth_enabled = !var.manage_app_credentials
    password_auth_enabled         = var.manage_app_credentials
    tenant_id                     = var.manage_app_credentials ? null : data.azurerm_client_config.current.tenant_id
  }

  dynamic "high_availability" {
    for_each = var.multi_zone ? [1] : []
    content {
      mode = "ZoneRedundant"
    }
  }

  lifecycle {
    precondition {
      # Azure PITR rides on automated backups; the platform minimum retention (7)
      # is also the PITR floor.
      condition     = !var.pitr || local.retention_days >= 7
      error_message = "pitr requires backup_retention_days >= 7 (Azure PITR rides on automated backups)."
    }
    # HA failover reassigns zones; pinning them would make every apply fight the
    # platform after a failover.
    ignore_changes = [zone, high_availability[0].standby_availability_zone]
  }
}

data "azurerm_client_config" "current" {}

resource "azurerm_postgresql_flexible_server" "replica" {
  count               = var.read_replicas
  name                = "${var.name_prefix}-pg-ro-${count.index}"
  location            = var.region
  resource_group_name = var.resource_group_name

  create_mode      = "Replica"
  source_server_id = azurerm_postgresql_flexible_server.this.id

  delegated_subnet_id           = var.delegated_subnet_id
  private_dns_zone_id           = var.private_dns_zone_id
  public_network_access_enabled = false

  lifecycle {
    ignore_changes = [zone]
  }
}

# Azure has no deletion_protection attribute on the server itself; a CanNotDelete
# management lock is the platform-native control (§10.1). It blocks deletes at the
# ARM layer until the lock itself is removed — which is exactly the two-step
# friction deletion protection is for.
resource "azurerm_management_lock" "this" {
  count      = var.deletion_protection ? 1 : 0
  name       = "${var.name_prefix}-pg-lock"
  scope      = azurerm_postgresql_flexible_server.this.id
  lock_level = "CanNotDelete"
  notes      = "prd preset: protect the database from deletion (neckbeard)"
}

resource "azurerm_postgresql_flexible_server_database" "app" {
  name      = "app"
  server_id = azurerm_postgresql_flexible_server.this.id
  charset   = "UTF8"
  collation = "en_US.utf8"
}
