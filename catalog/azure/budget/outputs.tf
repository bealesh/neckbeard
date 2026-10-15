output "budget_name" {
  description = "Name of the monthly consumption budget."
  value       = azurerm_consumption_budget_resource_group.this.name
}
