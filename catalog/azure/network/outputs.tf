output "app_subnet_id" {
  description = "Subnet delegated to Microsoft.App/environments; the runtime module injects the Container Apps environment here."
  value       = azurerm_subnet.apps.id
}

output "db_subnet_id" {
  description = "Subnet delegated to Microsoft.DBforPostgreSQL/flexibleServers; the postgres module injects the server here."
  value       = azurerm_subnet.db.id
}

output "postgres_dns_zone_id" {
  description = "Private DNS zone the flexible server registers its FQDN in."
  value       = azurerm_private_dns_zone.postgres.id
}
