variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}. The registry name derives from it (dashes stripped, truncated to 50) — see main.tf."
  type        = string
}

variable "region" {
  description = "Azure region, used as the location of every resource in this module."
  type        = string
}

variable "resource_group_name" {
  description = "Resource group all module resources land in (owned by the env root; modules never create resource groups)."
  type        = string
}
