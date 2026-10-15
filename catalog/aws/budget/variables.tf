variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "AWS region (informational; the provider sets the region)."
  type        = string
}

variable "app" {
  description = "Application name; matched against the neckbeard-app cost allocation tag."
  type        = string
}

variable "environment" {
  description = "Environment name; matched against the neckbeard-env cost allocation tag."
  type        = string
}

variable "monthly_amount" {
  description = "Monthly budget for this environment. Alerts fire at 80% and 100% actual and 100% forecasted; nothing is capped."
  type        = number
}

variable "currency" {
  description = "Informational: AWS Budgets are always USD."
  type        = string
  default     = "USD"
}

variable "alert_emails" {
  description = "Recipients of budget notifications."
  type        = list(string)
  validation {
    condition     = length(var.alert_emails) > 0
    error_message = "A budget with nobody to notify is theatre; set finops.alert_emails."
  }
}
