variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Azure region, used as the location of every resource in this module."
  type        = string
}

variable "zones" {
  description = "Number of availability zones to span. On Azure this is informational (honest per-cloud difference, DESIGN §3.3): subnets are regional, not zonal, so the VNet layout does not change with this number. Zone spread happens at the workload layer (Container Apps zone redundancy, Postgres zone-redundant HA)."
  type        = number
  validation {
    condition     = var.zones >= 1 && var.zones <= 3
    error_message = "zones must be between 1 and 3."
  }
}

variable "nat_strategy" {
  description = "Egress for the apps subnet: none (platform-default outbound only), single or per-zone (a NAT gateway on the apps subnet). On Azure, single and per-zone produce the same topology — see the comment in main.tf."
  type        = string
  validation {
    condition     = contains(["none", "single", "per-zone"], var.nat_strategy)
    error_message = "nat_strategy must be none, single, or per-zone."
  }
}

variable "resource_group_name" {
  description = "Resource group all module resources land in. The env root owns the group (azurerm_resource_group.this); modules never create resource groups."
  type        = string
}
