variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (informational; the provider sets the region)."
  type        = string
}
