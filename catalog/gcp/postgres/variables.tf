variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Cloud SQL region."
  type        = string
}

variable "instance_class" {
  description = "Catalog size class; mapped to a Cloud SQL tier."
  type        = string
  validation {
    condition     = contains(["smallest", "small", "medium"], var.instance_class)
    error_message = "instance_class must be smallest, small, or medium."
  }
}

variable "multi_zone" {
  description = "Regional (HA) availability instead of zonal."
  type        = bool
}

variable "read_replicas" {
  description = "Number of read replicas."
  type        = number
  default     = 0
}

variable "backup_retention_days" {
  description = "Automated backup retention (retained backup count on Cloud SQL)."
  type        = number
}

variable "pitr" {
  description = "Enable point-in-time recovery (requires backups)."
  type        = bool
}

variable "deletion_protection" {
  description = "Protect the instance from deletion (prd preset sets this)."
  type        = bool
  default     = false
}

variable "network_id" {
  description = "VPC network for the private IP (wired from the network module)."
  type        = string
}

variable "private_services_connection" {
  description = "Private services access connection id from the network module; consumed in a precondition to order Cloud SQL after the peering exists."
  type        = string
}
