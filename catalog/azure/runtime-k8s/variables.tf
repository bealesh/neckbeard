variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AKS location."
  type        = string
}

variable "node_shape" {
  description = "Catalog node size class; mapped to amd64 VM sizes (CI builds amd64 images)."
  type        = string
  validation {
    condition     = contains(["small", "medium"], var.node_shape)
    error_message = "node_shape must be small or medium."
  }
}

variable "min_nodes" {
  description = "Autoscaling floor for the workload pool."
  type        = number
}

variable "max_nodes" {
  description = "Autoscaling ceiling for the workload pool."
  type        = number
}

variable "spot" {
  description = "Add a Spot user pool for workloads (dev preset). AKS default pools cannot be Spot, so the default pool stays a 1-node system pool and workloads carry the standard Spot toleration (the delivery layer sets it on every k8s lane — a no-op where the taint never exists)."
  type        = bool
}

variable "resource_group_name" {
  description = "Resource group (created by the env root)."
  type        = string
}

variable "subnet_id" {
  description = "Undelegated AKS node subnet (wired from the network module)."
  type        = string
}

variable "key_vault_id" {
  description = "Key Vault for the external-secrets identity's role assignment (wired from the secrets module)."
  type        = string
  default     = ""
}

variable "registry_id" {
  description = "ACR resource id for the kubelet's AcrPull role (wired from the registry module)."
  type        = string
}
