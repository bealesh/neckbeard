variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (informational; the provider sets the region)."
  type        = string
}

variable "vpc_id" {
  description = "VPC for the load balancer (wired from the network module)."
  type        = string
}

variable "public_subnet_ids" {
  description = "Public subnets for the load balancer."
  type        = list(string)
}

variable "http_services" {
  description = "HTTP services to expose. The first (sorted by the planner) is the default route; the rest get path rules /<name>/*."
  type = list(object({
    name        = string
    port        = number
    health_path = string
  }))
}

variable "waf_enabled" {
  description = "Attach a WAFv2 web ACL (AWS managed common rule set) to the load balancer."
  type        = bool
}
