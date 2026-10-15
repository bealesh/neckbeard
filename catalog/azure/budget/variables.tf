variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Azure region (informational)."
  type        = string
}

variable "resource_group_id" {
  description = "Id of the environment's resource group the budget is scoped to."
  type        = string
}

variable "monthly_amount" {
  description = "Monthly budget for this environment in the subscription's billing currency. Alerts fire at 80% and 100% actual and 100% forecasted; nothing is capped."
  type        = number
}

variable "currency" {
  description = "Informational: Azure budgets use the subscription's billing currency."
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
