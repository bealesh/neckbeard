variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (informational; the provider sets the region)."
  type        = string
}

variable "environment" {
  description = "Environment this bootstrap serves (dev|stg|prd); OIDC subjects bind to it."
  type        = string
}

variable "vcs" {
  description = "CI provider the OIDC trust is established for."
  type        = string
  validation {
    condition     = contains(["github", "gitlab"], var.vcs)
    error_message = "vcs must be github or gitlab."
  }
}

variable "repo" {
  description = "Repository slug (owner/name on GitHub, group/project on GitLab). Trust conditions bind to exactly this repository."
  type        = string
}
