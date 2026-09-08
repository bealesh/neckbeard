variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Location for the state storage account and its resource group."
  type        = string
}

variable "environment" {
  description = "Environment this bootstrap serves (dev|stg|prd); federated subjects bind to it."
  type        = string
}

variable "vcs" {
  description = "CI provider the federated credentials trust."
  type        = string
  validation {
    condition     = contains(["github", "gitlab"], var.vcs)
    error_message = "vcs must be github or gitlab."
  }
}

variable "repo" {
  description = "Repository slug (owner/name on GitHub, group/project on GitLab). Federated subjects bind to exactly this repository."
  type        = string
}
