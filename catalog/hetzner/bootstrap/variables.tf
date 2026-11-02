variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Hetzner location; Object Storage exists in fsn1, nbg1 and hel1."
  type        = string
  validation {
    condition     = contains(["fsn1", "nbg1", "hel1"], var.region)
    error_message = "Hetzner Object Storage (the state backend) is offered in fsn1, nbg1 and hel1 only."
  }
}

variable "environment" {
  description = "Environment name (dev|stg|prd)."
  type        = string
}

variable "vcs" {
  description = "github | gitlab."
  type        = string
}

variable "repo" {
  description = "owner/name of the application repository."
  type        = string
}
