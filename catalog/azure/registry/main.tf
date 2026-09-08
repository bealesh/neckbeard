# Container registry. Admin user disabled: CI pushes and Container Apps pulls run
# on federated/managed identities, never static registry credentials (DESIGN
# §10.1). Honest per-cloud difference (§3.3): ACR has no per-tag immutability
# setting like ECR's IMMUTABLE mode at this SKU — the promotion flow doesn't rely
# on tags anyway, it records and moves image DIGESTS (§11.2), which are immutable
# by construction.

locals {
  # Registry names: 5-50 chars, alphanumerics only. Same truncation-collision
  # caveat as the storage account; the global namespace makes a collision a
  # create-time error.
  registry_name = substr(lower(replace(var.name_prefix, "-", "")), 0, 50)
}

resource "azurerm_container_registry" "this" {
  name                = local.registry_name
  location            = var.region
  resource_group_name = var.resource_group_name
  sku                 = "Basic"
  admin_enabled       = false
}
