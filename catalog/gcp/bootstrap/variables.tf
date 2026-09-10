variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
  validation {
    condition     = length(var.name_prefix) <= 25
    error_message = "name_prefix must be at most 25 characters so plan and apply service account IDs stay distinct; shorten org or app."
  }
}

variable "region" {
  description = "State bucket location."
  type        = string
}

variable "environment" {
  description = "Environment this bootstrap serves (dev|stg|prd)."
  type        = string
}

variable "vcs" {
  description = "CI provider the workload identity federation trusts."
  type        = string
  validation {
    condition     = contains(["github", "gitlab"], var.vcs)
    error_message = "vcs must be github or gitlab."
  }
}

variable "repo" {
  description = "Repository slug (owner/name on GitHub, group/project on GitLab). Federation conditions bind to exactly this repository."
  type        = string
}
