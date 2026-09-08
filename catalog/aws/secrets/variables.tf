variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (informational; the provider sets the region)."
  type        = string
}

variable "secret_names" {
  description = "Secret container names to provision. Values are set out-of-band by operators — neckbeard never writes or reads secret values."
  type        = list(string)
  default     = []
}
