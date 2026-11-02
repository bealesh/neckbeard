variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Hetzner location (informational)."
  type        = string
}

variable "vcs" {
  description = "github | gitlab — chooses ghcr.io or registry.gitlab.com."
  type        = string
  validation {
    condition     = contains(["github", "gitlab"], var.vcs)
    error_message = "vcs must be github or gitlab."
  }
}

variable "repo" {
  description = "owner/name of the application repository; the image path mirrors it."
  type        = string
}
