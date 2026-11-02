variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Hetzner location the volume (and server) live in."
  type        = string
}

variable "volume_gb" {
  description = "Data volume size in GB (minimum 10)."
  type        = number
  default     = 20
  validation {
    condition     = var.volume_gb >= 10
    error_message = "Hetzner volumes start at 10 GB."
  }
}

variable "backup_retention_days" {
  description = "Nightly pg_dump copies kept in object storage before the runtime prunes them."
  type        = number
  default     = 7
}

variable "deletion_protection" {
  description = "Refuse volume deletion from the API/console until lifted."
  type        = bool
  default     = false
}

# Accepted for catalog parity with the managed-Postgres modules; none of these
# exist on a single box and the planner refuses tiers that would need them.
variable "instance_class" {
  type    = string
  default = "smallest"
}

variable "multi_zone" {
  type    = bool
  default = false
  validation {
    condition     = var.multi_zone == false
    error_message = "Postgres on a Hetzner box has no standby; multi_zone is not available on this lane."
  }
}

variable "read_replicas" {
  type    = number
  default = 0
  validation {
    condition     = var.read_replicas == 0
    error_message = "Postgres on a Hetzner box has no read replicas."
  }
}

variable "pitr" {
  type    = bool
  default = false
  validation {
    condition     = var.pitr == false
    error_message = "Postgres on a Hetzner box has nightly dumps, not point-in-time recovery."
  }
}
