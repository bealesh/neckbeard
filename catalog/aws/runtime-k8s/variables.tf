variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (informational; the provider sets the region)."
  type        = string
}

variable "node_shape" {
  description = "Catalog node size class; mapped to amd64 instance types (CI builds amd64 images)."
  type        = string
  validation {
    condition     = contains(["small", "medium"], var.node_shape)
    error_message = "node_shape must be small or medium."
  }
}

variable "min_nodes" {
  description = "Node group minimum (also the initial desired size)."
  type        = number
}

variable "max_nodes" {
  description = "Node group autoscaling ceiling."
  type        = number
}

variable "spot" {
  description = "Use spot capacity (dev preset). Interruption-tolerant workloads only; the control plane is unaffected."
  type        = bool
}

variable "vpc_id" {
  description = "VPC (wired from the network module)."
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnets for nodes and the cluster ENIs."
  type        = list(string)
}
