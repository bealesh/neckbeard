variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Azure region, used as the location of every resource in this module."
  type        = string
}

variable "cpu" {
  description = "vCPU per replica (catalog sizes: 0.5, 1, 2). Container Apps consumption workloads accept fixed cpu/memory pairs — see the precondition in main.tf."
  type        = string
  validation {
    condition     = contains(["0.5", "1", "2"], var.cpu)
    error_message = "cpu must be one of 0.5, 1, 2."
  }
}

variable "memory_gb" {
  description = "Memory per replica in GB. Must pair with cpu per the Container Apps consumption table (0.5→1, 1→2, 2→4)."
  type        = number
}

variable "min_instances" {
  description = "Minimum replicas per service. 0 is honest scale-to-zero on Container Apps (unlike ECS, DESIGN §3.3) — except workers, see the comment in main.tf."
  type        = number
}

variable "max_instances" {
  description = "Scaling ceiling per service (Container Apps scaling is KEDA-based, §3.3)."
  type        = number
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
  default     = "mcr.microsoft.com/k8se/quickstart:latest"
}

variable "resource_group_name" {
  description = "Resource group all module resources land in (owned by the env root; modules never create resource groups)."
  type        = string
}

variable "subnet_id" {
  description = "Subnet delegated to Microsoft.App/environments; the Container Apps environment's infrastructure subnet (wired from the network module)."
  type        = string
}

variable "key_vault_id" {
  description = "Key Vault holding the app's secrets (wired from the secrets module). Empty when the plan has no secrets capability."
  type        = string
  default     = ""
}

variable "secret_uris" {
  description = "Map of secret name to versionless Key Vault secret URI (wired from the secrets module). Each becomes a Container Apps secret reference plus an env var. Values are set out-of-band by operators — neckbeard never writes or reads secret values (DESIGN §3.1, §10.1)."
  type        = map(string)
  default     = {}
}

variable "registry_server" {
  description = "ACR login server the apps pull from (wired from the registry module)."
  type        = string
}

variable "registry_id" {
  description = "ACR resource id for the pull identity's AcrPull role (wired from the registry module)."
  type        = string
}
