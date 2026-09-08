variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "GCP region for the subnet, NAT, and private services access."
  type        = string
}

variable "zones" {
  description = "Zone count from the tier preset. Informational on GCP: subnets are regional and Cloud Run spreads across zones automatically — an honest per-cloud difference (DESIGN §3.3)."
  type        = number
}

variable "nat_strategy" {
  description = "Egress for VPC-routed traffic: none (no Cloud NAT), single / per-zone (one Cloud NAT — Cloud NAT is regional, so both map to one; documented difference)."
  type        = string
  validation {
    condition     = contains(["none", "single", "per-zone"], var.nat_strategy)
    error_message = "nat_strategy must be none, single, or per-zone."
  }
}
