output "cluster_name" {
  value = azurerm_kubernetes_cluster.this.name
}

output "cluster_endpoint" {
  value = azurerm_kubernetes_cluster.this.fqdn
}

output "oidc_issuer_url" {
  value = azurerm_kubernetes_cluster.this.oidc_issuer_url
}

output "external_secrets_client_id" {
  description = "Bootstrap publishes this into the neckbeard-cluster-vars ConfigMap (Flux postBuild substitution completes the controllers layer)."
  value       = azurerm_user_assigned_identity.external_secrets.client_id
}
