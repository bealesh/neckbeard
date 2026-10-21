variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "GCP region (informational; the provider sets the region)."
  type        = string
}

variable "billing_account" {
  description = "Billing account id (XXXXXX-XXXXXX-XXXXXX) the project is attached to; budgets live on the billing account, not the project."
  type        = string
}

variable "monthly_amount" {
  description = "Monthly budget for this environment. Alerts fire at 80% and 100% actual and 100% forecasted; nothing is capped."
  type        = number
}

variable "currency" {
  description = "Currency code of the budget amount; must match the billing account's currency."
  type        = string
  default     = "USD"
}

variable "alert_emails" {
  description = "Recipients of budget notifications (email notification channels are created for each)."
  type        = list(string)
  validation {
    condition     = length(var.alert_emails) > 0
    error_message = "A budget with nobody to notify is theatre; set finops.alert_emails."
  }
}
