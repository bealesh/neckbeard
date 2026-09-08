variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Artifact Registry location."
  type        = string
}
