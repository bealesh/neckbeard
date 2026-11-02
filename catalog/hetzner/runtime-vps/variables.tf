variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Hetzner location (fsn1, nbg1, hel1, ash, hil, sin)."
  type        = string
}

variable "server_shape" {
  description = "small (2 vCPU / 4 GB), medium (4 / 8), large (8 / 16); maps to cx23/cx33/cx43 or cax11/cax21/cax31 for arm."
  type        = string
  default     = "small"
  validation {
    condition     = contains(["small", "medium", "large"], var.server_shape)
    error_message = "server_shape must be small, medium or large."
  }
}

variable "arch" {
  description = "x86 or arm. The image must be built for it; arm is cheaper per core."
  type        = string
  default     = "x86"
  validation {
    condition     = contains(["x86", "arm"], var.arch)
    error_message = "arch must be x86 or arm."
  }
}

variable "services" {
  description = "Service definitions from the blueprint. schedule is 5-field cron for kind=cron."
  type = list(object({
    name        = string
    kind        = string # http | worker | cron
    port        = number
    health_path = string
    schedule    = string
    args        = list(string)
  }))
}

variable "image" {
  description = "Container image for all services. PLACEHOLDER DEFAULT for validate/plan before the first release: the per-env release state owns the real digest (DESIGN §11.2, D3)."
  type        = string
  default     = "docker.io/library/busybox:stable"
}

variable "environment" {
  description = "Environment name exposed to the app as APP_ENV."
  type        = string
}

variable "network_id" {
  description = "Private network (wired from the network module)."
  type        = string
}

variable "server_firewall_id" {
  description = "Public-interface firewall (wired from the network module)."
  type        = string
}

variable "secret_names" {
  description = "Secret names the app reads; written as an env-file template, values filled by the operator on the box."
  type        = list(string)
  default     = []
}

variable "registry_host" {
  description = "Registry the server pulls from (wired from the registry module)."
  type        = string
}

variable "data_volume_id" {
  description = "Block-storage volume for on-box Postgres (wired from the postgres module); null means no database on this box."
  type        = string
  default     = null
}

variable "backup_bucket_name" {
  description = "Object Storage bucket for nightly database dumps (wired from the storage module)."
  type        = string
  default     = ""
}

variable "object_storage_endpoint" {
  description = "Object Storage endpoint host (wired from the storage module)."
  type        = string
  default     = ""
}

variable "backup_retention_days" {
  description = "Days of nightly dumps kept (wired from the postgres module)."
  type        = number
  default     = 7
}

variable "ssh_key_ids" {
  description = "hcloud SSH key ids installed on the server. Empty means console access only until the operator adds one."
  type        = list(string)
  default     = []
}

variable "deletion_protection" {
  description = "Protect the server from deletion and rebuild via the API/console."
  type        = bool
  default     = false
}

# Accepted for parity with the serverless lanes' tier presets; a single box has
# no instance range. min_instances > 1 or max_instances > 1 is refused rather
# than silently flattened.
variable "min_instances" {
  type    = number
  default = 1
}

variable "max_instances" {
  type    = number
  default = 1
  validation {
    condition     = var.max_instances <= 1
    error_message = "A Hetzner box runs one instance of each service; horizontal scaling is not on this lane."
  }
}
