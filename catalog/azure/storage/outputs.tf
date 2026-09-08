output "account_name" {
  description = "Storage account name (derived from name_prefix: dashes stripped, truncated to 24 chars)."
  value       = azurerm_storage_account.data.name
}

output "account_id" {
  description = "Storage account id; app-scoped role assignments attach at the env root."
  value       = azurerm_storage_account.data.id
}
