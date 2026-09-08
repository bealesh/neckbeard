variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}. Bucket names derive from it; global uniqueness rides on org uniqueness."
  type        = string
}

variable "region" {
  description = "Bucket location."
  type        = string
}

variable "versioning" {
  description = "Enable object versioning."
  type        = bool
}
