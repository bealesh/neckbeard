variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Hetzner location for the load balancer."
  type        = string
}

variable "network_id" {
  description = "Private network (wired from the network module)."
  type        = string
}

variable "server_id" {
  description = "Application server to target (wired from the runtime module)."
  type        = string
}

variable "http_services" {
  description = "HTTP services to expose. The first (sorted by the planner) listens on 443; the rest on 444, 445, ..."
  type = list(object({
    name        = string
    port        = number
    health_path = string
  }))
}

variable "load_balancer_type" {
  description = "lb11 (default), lb21, lb31."
  type        = string
  default     = "lb11"
}

variable "public_hostname" {
  description = "Hostname for HTTPS. Null until the operator delegates DNS; then the load balancer gets a managed certificate and an A record."
  type        = string
  default     = null
}

variable "zone_name" {
  description = "DNS zone the hostname belongs to (e.g. hz.example.com). Required with public_hostname."
  type        = string
  default     = ""
}

variable "manage_zone" {
  description = "Create the zone in Hetzner DNS (true, the default: delegate NS records to the zone's authoritative_nameservers) or use an existing Hetzner DNS zone (false)."
  type        = bool
  default     = true
}

variable "waf_enabled" {
  description = "Accepted for catalog parity; Hetzner load balancers have no WAF. true is refused rather than ignored."
  type        = bool
  default     = false
  validation {
    condition     = var.waf_enabled == false
    error_message = "Hetzner load balancers have no managed WAF; waf_enabled must be false on this lane."
  }
}
