output "key_vault_id" {
  description = "Key Vault id; the runtime module scopes 'Key Vault Secrets User' role assignments to it."
  value       = azurerm_key_vault.this.id
}

output "secret_uris" {
  description = "Map of declared secret name to its computed VERSIONLESS secret URI. Key Vault secret names cannot contain underscores, so DATABASE_URL lives in the vault as DATABASE-URL — the map key keeps the original spelling for env var wiring. The secrets do not exist until operators set values out-of-band (never via neckbeard); Container Apps resolves these URIs at app create time."
  value       = { for name in var.secret_names : name => "${azurerm_key_vault.this.vault_uri}secrets/${replace(name, "_", "-")}" }
}
