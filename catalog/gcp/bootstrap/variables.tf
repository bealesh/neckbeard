variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
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
