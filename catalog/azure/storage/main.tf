# Application object storage: private, TLS 1.2+, public blob access off
# (DESIGN §10.1 — org guardrails additionally deny public blobs on Path F).

locals {
  # Storage account names allow only lowercase alphanumerics, max 24 chars: strip
  # the prefix's dashes and truncate. Truncation means two long org/app/env
  # combinations that only differ past 24 characters would collide — the global
  # namespace makes that a create-time error, not silent reuse, and the fix is a
  # shorter org/app name.
  account_name = substr(lower(replace(var.name_prefix, "-", "")), 0, 24)
}

resource "azurerm_storage_account" "data" {
  name                = local.account_name
  location            = var.region
  resource_group_name = var.resource_group_name

  # LRS matches the catalog's cost floor; storage redundancy is not preset-wired
  # at M1 (the tiers' availability objective governs compute/DB zones, and blob
  # redundancy upgrades are an online change when a catalog input lands).
  account_tier             = "Standard"
  account_replication_type = "LRS"
  min_tls_version          = "TLS1_2"
  # Entra-only data plane: no shared keys, hence no account SAS either — the
  # backend/apps authenticate with Azure AD (no-static-keys, §10.1).
  shared_access_key_enabled = false

  # Private by default: no anonymous blob access anywhere in this account.
  allow_nested_items_to_be_public = false

  blob_properties {
    versioning_enabled = var.versioning
  }
}

resource "azurerm_storage_container" "data" {
  name                  = "data"
  storage_account_id    = azurerm_storage_account.data.id
  container_access_type = "private"
}
