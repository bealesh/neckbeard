output "login_server" {
  description = "Registry login server (images are addressed as <login_server>/<repo>@<digest>, DESIGN §11.2)."
  value       = azurerm_container_registry.this.login_server
}

output "registry_id" {
  description = "Registry id; AcrPull role assignments for the runtime identities attach at the env root."
  value       = azurerm_container_registry.this.id
}
