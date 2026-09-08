variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "GCP region (informational; secrets use automatic replication)."
  type        = string
}

variable "secret_names" {
  description = "Secret container names to provision. Values are set out-of-band by operators — neckbeard never writes or reads secret values."
  type        = list(string)
  default     = []
}
