variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Hetzner location (informational)."
  type        = string
}

variable "secret_names" {
  description = "Secret names the app reads. Values are never set here."
  type        = list(string)
  default     = []
}
