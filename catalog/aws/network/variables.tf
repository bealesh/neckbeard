variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (informational; the provider sets the region)."
  type        = string
}

variable "zones" {
  description = "Number of availability zones to span."
  type        = number
  validation {
    condition     = var.zones >= 1 && var.zones <= 3
    error_message = "zones must be between 1 and 3."
  }
}

variable "nat_strategy" {
  description = "Egress for private subnets: none (no egress), single (one NAT gateway), per-zone (one per AZ)."
  type        = string
  validation {
    condition     = contains(["none", "single", "per-zone"], var.nat_strategy)
    error_message = "nat_strategy must be none, single, or per-zone."
  }
}
