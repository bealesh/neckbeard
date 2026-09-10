variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Azure region, used as the location of every resource in this module."
  type        = string
}

variable "instance_class" {
  description = "Catalog size class; mapped to a flexible-server SKU. Honest per-cloud difference (DESIGN §3.3): when multi_zone is true, Azure requires a GeneralPurpose (or better) SKU for zone-redundant HA, so smallest/small are forced up to GP_Standard_D2ds_v4 — see main.tf."
  type        = string
  validation {
    condition     = contains(["smallest", "small", "medium"], var.instance_class)
    error_message = "instance_class must be smallest, small, or medium."
  }
}

variable "multi_zone" {
  description = "Zone-redundant HA (drives the availability objective). Forces a GeneralPurpose SKU on Azure — Burstable SKUs cannot run zone-redundant HA."
  type        = bool
}

variable "read_replicas" {
  description = "Number of read replicas."
  type        = number
  default     = 0
}

variable "backup_retention_days" {
  description = "Automated backup retention. Azure flexible server accepts 7-35 days; presets below 7 (dev asks for 3) are clamped up to Azure's minimum of 7 — an honest per-cloud difference (DESIGN §3.3), the cost report prices the clamped value."
  type        = number
  validation {
    condition     = var.backup_retention_days >= 1 && var.backup_retention_days <= 35
    error_message = "backup_retention_days must be between 1 and 35 (values below 7 are clamped to Azure's minimum of 7)."
  }
}

variable "pitr" {
  description = "Require point-in-time recovery. Azure PITR rides on automated backups; enforced as retention >= 7 by the precondition in main.tf."
  type        = bool
}

variable "deletion_protection" {
  description = "Protect the server from deletion (prd preset sets this). Azure has no per-resource deletion-protection flag; this places a CanNotDelete management lock on the server — the Azure-native equivalent (honest difference, DESIGN §3.3)."
  type        = bool
  default     = false
}

variable "resource_group_name" {
  description = "Resource group all module resources land in (owned by the env root; modules never create resource groups)."
  type        = string
}

variable "delegated_subnet_id" {
  description = "Subnet delegated to Microsoft.DBforPostgreSQL/flexibleServers (wired from the network module); the server is VNet-injected with no public endpoint."
  type        = string
}

variable "private_dns_zone_id" {
  description = "Private DNS zone for the server's FQDN (wired from the network module)."
  type        = string
}

variable "manage_app_credentials" {
  description = "Provision password authentication using an ephemeral, write-only input."
  type        = bool
  default     = false
}
variable "application_password" {
  description = "Fetched from the cloud secret store by the deployment operator; never persisted in plans/state."
  type        = string
  sensitive   = true
  ephemeral   = true
  default     = null
}
