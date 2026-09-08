variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (used for CloudWatch log configuration)."
  type        = string
}

variable "cpu" {
  description = "vCPU per task (catalog sizes: 0.5, 1, 2)."
  type        = string
  validation {
    condition     = contains(["0.5", "1", "2"], var.cpu)
    error_message = "cpu must be one of 0.5, 1, 2."
  }
}

variable "memory_gb" {
  description = "Memory per task in GB."
  type        = number
}

variable "min_instances" {
  description = "Minimum running tasks per service (0 = worker/http may idle at zero; ECS still bills running tasks only)."
  type        = number
}

variable "max_instances" {
  description = "Autoscaling ceiling per service."
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
  }))
}

variable "image" {
  description = "Container image for all services. PLACEHOLDER DEFAULT for validate/plan before the first release: the per-env release state owns the real digest (DESIGN §11.2, D3)."
  type        = string
  default     = "public.ecr.aws/docker/library/busybox:stable"
}

variable "vpc_id" {
  description = "VPC (wired from the network module)."
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnets for tasks."
  type        = list(string)
}

variable "alb_security_group_id" {
  description = "Ingress ALB security group allowed to reach http services (wired from dns-ingress)."
  type        = string
  default     = ""
}

variable "target_group_arns" {
  description = "Map of http service name to ALB target group ARN (wired from dns-ingress)."
  type        = map(string)
  default     = {}
}

variable "secret_arns" {
  description = "Map of secret name to ARN; each becomes a container secret env var (wired from the secrets module)."
  type        = map(string)
  default     = {}
}
