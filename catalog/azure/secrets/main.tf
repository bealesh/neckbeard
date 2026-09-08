# Key Vault only — deliberately NO azurerm_key_vault_secret resources. Honest
# per-cloud difference (DESIGN §3.3): AWS/GCP have value-less secret *containers*
# to provision; a Key Vault secret resource cannot exist without a value, and that
# value would be written into OpenTofu state (§10.1, §10.3: state is protected as
# a secret, but must not be handed secrets it never needed). So the vault is the
# provisioned container, and the declared names surface as computed versionless
# URIs (outputs.tf) that operators fill out-of-band before the first runtime apply.

data "azurerm_client_config" "current" {}

locals {
  # Vault names: 3-24 chars, alphanumerics and dashes, must start with a letter.
  # name_prefix is {org}-{app}-{env}; truncate and trim a dangling dash. Same
  # truncation-collision caveat as the storage account: the global vault namespace
  # turns a collision into a create-time error, never silent reuse.
  vault_name = trimsuffix(substr(var.name_prefix, 0, 24), "-")
}

resource "azurerm_key_vault" "this" {
  name                = local.vault_name
  location            = var.region
  resource_group_name = var.resource_group_name
  tenant_id           = data.azurerm_client_config.current.tenant_id
  sku_name            = "standard"

  # RBAC, not access policies: app identities get "Key Vault Secrets User" role
  # assignments (runtime module) — same least-privilege model as the other lanes.
  enable_rbac_authorization = true

  # Purge protection stays OFF at M1: the release matrix's deploy→verify→teardown
  # loop (DESIGN §12) must be able to delete and recreate environments freely, and
  # purge protection makes a vault name unusable for up to 90 days after delete.
  # Revisit for prd once teardown-ability stops being a daily requirement.
  purge_protection_enabled   = false
  soft_delete_retention_days = 7

  # The vault keeps its public endpoint at M1 (data plane still gated by RBAC +
  # Entra): Container Apps resolves Key Vault references over that endpoint and is
  # not on Key Vault's trusted-services list, so a default-deny firewall or
  # private-endpoint-only vault would break secret resolution until a private
  # endpoint + zone land in the catalog. Honest gap, tracked for post-M1.
}

# The deploying operator seeds secret VALUES out-of-band (DESIGN §8) — which needs
# a data-plane role: RBAC-mode vaults grant nothing implicitly, Owner included
# (V2 finding, 2026-09-08). Org-managed operator groups replace this on Path F.
resource "azurerm_role_assignment" "deployer_secrets_officer" {
  scope                = azurerm_key_vault.this.id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = data.azurerm_client_config.current.object_id
}
