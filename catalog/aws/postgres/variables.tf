variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (informational; the provider sets the region)."
  type        = string
}

variable "instance_class" {
  description = "Catalog size class; mapped to an RDS instance class."
  type        = string
  validation {
    condition     = contains(["smallest", "small", "medium"], var.instance_class)
    error_message = "instance_class must be smallest, small, or medium."
  }
}

variable "multi_zone" {
  description = "Multi-AZ standby (drives the availability objective)."
  type        = bool
}

variable "read_replicas" {
  description = "Number of read replicas."
  type        = number
  default     = 0
}

variable "backup_retention_days" {
  description = "Automated backup retention. Retention > 0 enables RDS point-in-time recovery."
  type        = number
}

variable "pitr" {
  description = "Require point-in-time recovery (backup_retention_days must be > 0)."
  type        = bool
}

variable "deletion_protection" {
  description = "Protect the instance from deletion (prd preset sets this)."
  type        = bool
  default     = false
}

variable "vpc_id" {
  description = "VPC to place the database in (wired from the network module)."
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnets for the DB subnet group."
  type        = list(string)
}

variable "allowed_security_group_ids" {
  description = "Security groups allowed to reach Postgres on 5432 (the runtime's service SG)."
  type        = list(string)
  default     = []
}

variable "manage_app_credentials" {
  description = "Use an application-owned cloud password through a write-only input instead of RDS-managed rotation."
  type        = bool
  default     = false
}

variable "application_password" {
  description = "Cloud-stored application password; never persisted in plans/state."
  type        = string
  sensitive   = true
  ephemeral   = true
  default     = null
}
