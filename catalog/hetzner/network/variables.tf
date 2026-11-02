variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Hetzner location (fsn1, nbg1, hel1, ash, hil, sin)."
  type        = string
}

variable "network_zone" {
  description = "Hetzner network zone the location belongs to (eu-central, us-east, us-west, ap-southeast)."
  type        = string
}

variable "ip_range" {
  description = "Private network CIDR for this environment."
  type        = string
  default     = "10.0.0.0/16"
}

variable "ssh_allowed_cidrs" {
  description = "CIDRs allowed to reach port 22 on application servers. Empty means no SSH ingress; set it per environment when the operator needs a shell."
  type        = list(string)
  default     = []
}

variable "zones" {
  description = "Accepted for catalog parity; Hetzner locations are single-zone, so this is informational."
  type        = number
  default     = 1
}

variable "nat_strategy" {
  description = "Accepted for catalog parity; servers have their own public IPv4 and need no NAT."
  type        = string
  default     = "none"
}
