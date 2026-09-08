variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}. The storage account name derives from it (dashes stripped, truncated to 24) — global uniqueness rides on org uniqueness, see main.tf."
  type        = string
}

variable "region" {
  description = "Azure region, used as the location of every resource in this module."
  type        = string
}

variable "versioning" {
  description = "Enable blob versioning."
  type        = bool
}

variable "resource_group_name" {
  description = "Resource group all module resources land in (owned by the env root; modules never create resource groups)."
  type        = string
}
