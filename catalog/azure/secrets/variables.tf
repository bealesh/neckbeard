variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}. The vault name derives from it (truncated to 24 chars) — see main.tf."
  type        = string
}

variable "region" {
  description = "Azure region, used as the location of every resource in this module."
  type        = string
}

variable "secret_names" {
  description = "Declared secret names. On Azure these do NOT become azurerm_key_vault_secret resources — a secret resource carries its value, and that value would land in OpenTofu state. Only computed versionless URIs are emitted; operators set the values out-of-band (DESIGN §3.1, §10.1)."
  type        = list(string)
  default     = []
}

variable "resource_group_name" {
  description = "Resource group all module resources land in (owned by the env root; modules never create resource groups)."
  type        = string
}
