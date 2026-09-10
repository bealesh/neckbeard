# Bootstrap (DESIGN §10.3), Azure flavor: state storage first, then Entra app
# registrations with federated credentials for CI (no client secrets ever, §10.1),
# then subscription-scoped role assignments. This root keeps LOCAL state; the
# runbook treats its state file as an operator artifact.
#
# The bootstrap resource group holds only bootstrap-owned resources; env roots
# create their own.

data "azurerm_client_config" "current" {}
data "azurerm_subscription" "current" {}

resource "azurerm_resource_group" "bootstrap" {
  name     = "${var.name_prefix}-bootstrap"
  location = var.region
}

# --- 1. state backend ---

locals {
  # Storage account names: lowercase alnum, ≤24. "<prefix-sans-dashes>tf" keeps it
  # distinguishable from the app data account.
  state_account = substr("${lower(replace(var.name_prefix, "-", ""))}tf", 0, 24)
}

resource "azurerm_storage_account" "tfstate" {
  name                     = local.state_account
  location                 = var.region
  resource_group_name      = azurerm_resource_group.bootstrap.name
  account_tier             = "Standard"
  account_replication_type = "LRS"
  min_tls_version          = "TLS1_2"
  # Entra-only data plane: no shared keys, hence no account SAS either — the
  # backend/apps authenticate with Azure AD (no-static-keys, §10.1).
  shared_access_key_enabled       = false
  allow_nested_items_to_be_public = false

  blob_properties {
    versioning_enabled = true
    delete_retention_policy {
      days = 7
    }
  }
}

resource "azurerm_storage_container" "tfstate" {
  depends_on            = [azurerm_role_assignment.state_rw_deployer]
  name                  = "tfstate"
  storage_account_id    = azurerm_storage_account.tfstate.id
  container_access_type = "private"
}

# --- 2. federated CI identities (plan + apply) ---

locals {
  github                = var.vcs == "github"
  github_subject_prefix = coalesce(var.github_subject_prefix, "repo:${var.repo}")
  issuer                = local.github ? "https://token.actions.githubusercontent.com" : "https://gitlab.com"
  tier                  = { dev = "development", stg = "staging", prd = "production" }[var.environment]
  # GitHub uses repository/environment/ref; GitLab includes branch protection,
  # deployment tier, and environment protection in its customized subject.
  gitlab_suffix = ":ref_protected:true:deployment_tier:${local.tier}:environment_protected:${var.environment == "prd" ? "true" : "false"}"
  apply_subject = local.github ? "${local.github_subject_prefix}:environment:${var.environment}:ref:refs/heads/main" : "project_path:${var.repo}:ref_type:branch:ref:main${local.gitlab_suffix}"
  plan_subject  = local.github ? "${local.github_subject_prefix}:environment:${var.environment}-plan:ref:*" : "project_path:${var.repo}:ref_type:*:ref:*:ref_protected:*:deployment_tier:${local.tier}:environment_protected:*"
  audience      = local.github ? "api://AzureADTokenExchange" : "https://gitlab.com"
}


resource "azuread_application" "ci" {
  for_each     = toset(["plan", "apply"])
  display_name = "${var.name_prefix}-ci-${each.value}"
}

resource "azuread_service_principal" "ci" {
  for_each  = azuread_application.ci
  client_id = each.value.client_id
}

resource "azuread_application_federated_identity_credential" "ci" {
  for_each       = azuread_application.ci
  application_id = each.value.id
  display_name   = "${var.vcs}-${var.environment}"
  issuer         = local.issuer
  audiences      = [local.audience]
  subject        = local.apply_subject
}

# --- 3. role assignments ---
# Which identity a job uses is workflow logic; prd is additionally gated by
# environment protection / the protected branch (§11.3).

resource "azurerm_role_assignment" "plan_reader" {
  scope                = data.azurerm_subscription.current.id
  role_definition_name = "Reader"
  principal_id         = azuread_service_principal.ci["plan"].object_id
}

resource "azurerm_role_assignment" "state_rw" {
  for_each             = azuread_service_principal.ci
  scope                = azurerm_storage_account.tfstate.id
  role_definition_name = each.key == "plan" ? "Storage Blob Data Reader" : "Storage Blob Data Contributor"
  principal_id         = each.value.object_id
}

# The human running bootstrap also needs data-plane access to the state (Owner
# grants management plane only): local plans and break-glass applies depend on it.
resource "azurerm_role_assignment" "state_rw_deployer" {
  scope                = azurerm_storage_account.tfstate.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = data.azurerm_client_config.current.object_id
}

# Contributor cannot grant RBAC, and the catalog's runtime modules create role
# assignments (Key Vault Secrets User) — hence the RBAC Administrator pairing.
# Least-privilege refinement is roadmap work the release harness will shape.
resource "azurerm_role_assignment" "apply" {
  for_each = toset([
    "Contributor",
    "Role Based Access Control Administrator",
  ])
  scope                = data.azurerm_subscription.current.id
  role_definition_name = each.value
  principal_id         = azuread_service_principal.ci["apply"].object_id
}

# Exact main credentials retain their original addresses. This additional read-
# only credential supports PR/MR refs without widening the apply identity.
resource "azuread_application_flexible_federated_identity_credential" "plan" {
  application_id             = azuread_application.ci["plan"].id
  display_name               = "${var.vcs}-${var.environment}-plan"
  issuer                     = local.issuer
  audience                   = local.audience
  claims_matching_expression = "claims['sub'] matches '${local.plan_subject}'"
}
