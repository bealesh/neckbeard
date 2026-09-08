output "service_fqdns" {
  description = "Map of http service name to its Container Apps ingress FQDN (built-in HTTPS)."
  value = { for name, a in azurerm_container_app.this :
    name => a.ingress[0].fqdn if contains(keys(local.http_services), name)
  }
}

output "environment_id" {
  description = "Container Apps environment id."
  value       = azurerm_container_app_environment.this.id
}

output "principal_ids" {
  description = "System-assigned identity principal ids per app; app-scoped grants (e.g. storage) attach at the env root."
  value       = { for name, a in azurerm_container_app.this : name => a.identity[0].principal_id }
}
