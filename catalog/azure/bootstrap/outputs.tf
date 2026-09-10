output "state_account" {
  value = azurerm_storage_account.tfstate.name
}

output "release_store" {
  description = "Versioned storage for verified deployment and rollback receipts."
  value       = "${azurerm_storage_account.tfstate.primary_blob_endpoint}${azurerm_storage_container.tfstate.name}"
}

output "backend_hcl" {
  description = "Contents for infra/envs/<env>/backend.hcl (the pipelines init with it)."
  value       = <<-EOT
    resource_group_name  = "${azurerm_resource_group.bootstrap.name}"
    storage_account_name = "${azurerm_storage_account.tfstate.name}"
    container_name       = "${azurerm_storage_container.tfstate.name}"
    key                  = "envs/${var.environment}/terraform.tfstate"
    use_azuread_auth     = true
  EOT
}

output "ci_variables" {
  description = "CI variables the generated pipelines expect (set via gh/glab, see docs/bootstrap.md)."
  value = {
    "NECKBEARD_AZURE_TENANT_ID"                              = data.azurerm_client_config.current.tenant_id
    "NECKBEARD_AZURE_SUBSCRIPTION_${upper(var.environment)}" = data.azurerm_client_config.current.subscription_id
    "NECKBEARD_AZURE_PLAN_CLIENT_${upper(var.environment)}"  = azuread_application.ci["plan"].client_id
    "NECKBEARD_AZURE_APPLY_CLIENT_${upper(var.environment)}" = azuread_application.ci["apply"].client_id
  }
}

output "github_subject_prefix" {
  value = local.github ? local.github_subject_prefix : null
}
