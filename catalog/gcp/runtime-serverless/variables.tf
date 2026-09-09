variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Cloud Run region."
  type        = string
}

variable "cpu" {
  description = "vCPU per instance (catalog sizes: 0.5, 1, 2)."
  type        = string
  validation {
    condition     = contains(["0.5", "1", "2"], var.cpu)
    error_message = "cpu must be one of 0.5, 1, 2."
  }
}

variable "memory_gb" {
  description = "Memory per instance in GB."
  type        = number
}

variable "min_instances" {
  description = "Minimum instances per service. Cloud Run scales to zero — min 0 costs nothing while idle, an honest difference from ECS (DESIGN §3.3)."
  type        = number
}

variable "max_instances" {
  description = "Autoscaling ceiling per service."
  type        = number
}

variable "services" {
  description = "Service definitions from the blueprint. schedule is 5-field cron for kind=cron (Cloud Scheduler accepts it natively)."
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
  description = "Container image for all services. PLACEHOLDER DEFAULT for validate/plan before the first release: the release flow owns the real digest (DESIGN §11.2)."
  type        = string
  default     = "us-docker.pkg.dev/cloudrun/container/hello"
}

variable "subnet_id" {
  description = "Subnet for direct VPC egress (reach Cloud SQL private IP); wired from the network module."
  type        = string
}

variable "secret_ids" {
  description = "Map of logical secret name to Secret Manager secret id; each becomes a secret-backed env var (wired from the secrets module)."
  type        = map(string)
  default     = {}
}
