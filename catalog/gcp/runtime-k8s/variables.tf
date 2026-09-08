variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "GKE region (regional cluster)."
  type        = string
}

variable "node_shape" {
  description = "Catalog node size class; mapped to amd64 machine types (CI builds amd64 images)."
  type        = string
  validation {
    condition     = contains(["small", "medium"], var.node_shape)
    error_message = "node_shape must be small or medium."
  }
}

variable "min_nodes" {
  description = "Autoscaling floor (total across the region, not per zone)."
  type        = number
}

variable "max_nodes" {
  description = "Autoscaling ceiling (total across the region)."
  type        = number
}

variable "spot" {
  description = "Use spot capacity (dev preset)."
  type        = bool
}

variable "network_id" {
  description = "VPC network (wired from the network module)."
  type        = string
}

variable "subnet_id" {
  description = "Subnet for nodes (wired from the network module)."
  type        = string
}

variable "secret_ids" {
  description = "Map of logical secret name to Secret Manager secret id; external-secrets' identity gets accessor on exactly these (wired from the secrets module)."
  type        = map(string)
  default     = {}
}
