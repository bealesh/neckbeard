# Managed PostgreSQL (Azure Database for PostgreSQL – Flexible Server), private
# access only: VNet-injected into the delegated subnet, FQDN resolvable only via
# the private DNS zone (DESIGN §10.1). Authentication is Entra-only with password
# auth DISABLED, so no database password ever exists in OpenTofu state or version
# control — the app's DATABASE_URL connection secret is operator-provided
# out-of-band, like every secret value (§3.1).

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

  version    = "16"
  sku_name   = local.sku_name
  storage_mb = 32768

  delegated_subnet_id           = var.delegated_subnet_id
  private_dns_zone_id           = var.private_dns_zone_id
  public_network_access_enabled = false

  backup_retention_days = local.retention_days

  # Entra (AAD) authentication only: password auth is off, so the provider stores
  # no administrator password and none reaches state. Operators grant the app's
  # managed identity (or a bootstrap principal) access post-provision and set
  # DATABASE_URL as an out-of-band secret value.
  authentication {
    active_directory_auth_enabled = true
    password_auth_enabled         = false
    tenant_id                     = data.azurerm_client_config.current.tenant_id
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
