output "fqdn" {
  description = "Server FQDN (resolves only inside VNets linked to the private DNS zone; credentials: operator-set secret)."
  value       = azurerm_postgresql_flexible_server.this.fqdn
}

output "replica_fqdns" {
  description = "Read replica FQDNs."
  value       = azurerm_postgresql_flexible_server.replica[*].fqdn
}
