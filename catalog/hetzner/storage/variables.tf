variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Hetzner location; Object Storage exists in fsn1, nbg1 and hel1."
  type        = string
  validation {
    condition     = contains(["fsn1", "nbg1", "hel1"], var.region)
    error_message = "Hetzner Object Storage is offered in fsn1, nbg1 and hel1 only."
  }
}

variable "versioning" {
  description = "Keep previous object versions in the data bucket."
  type        = bool
  default     = false
}
