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

variable "existing_oidc_provider_arn" {
  description = "Reuse an account-wide OIDC provider owned by another bootstrap root."
  type        = string
  default     = null
}

variable "github_subject_prefix" {
  description = "Exact sub_claim_prefix from GitHub's repository OIDC API, including immutable IDs for new repositories. Null retains legacy name-based subjects."
  type        = string
  default     = null
  validation {
    condition = var.github_subject_prefix == null ? true : (
      can(regex("^repo:[A-Za-z0-9_.-]+(@[0-9]+)?/[A-Za-z0-9_.-]+(@[0-9]+)?$", var.github_subject_prefix)) &&
      replace(trimprefix(var.github_subject_prefix, "repo:"), "/@[0-9]+/", "") == var.repo
    )
    error_message = "github_subject_prefix must identify exactly this repository, with optional numeric immutable IDs and no wildcards."
  }
}
