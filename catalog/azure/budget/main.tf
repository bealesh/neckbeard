# Monthly consumption budget scoped to this environment's resource group.
# Budgets NOTIFY; they do not cap spend (DESIGN §10.2). The only hard ceiling
# neckbeard offers is finops.monthly_ceiling, enforced before anything exists.
#
# Azure budgets are denominated in the subscription's billing currency;
# finops.currency is informational here. The budget's start date must be the
# first of a month, so it is taken from the first plan and then ignored, which
# keeps later plans from proposing a replacement every month.

resource "azurerm_consumption_budget_resource_group" "this" {
  name              = "${var.name_prefix}-monthly"
  resource_group_id = var.resource_group_id
  amount            = var.monthly_amount
  time_grain        = "Monthly"

  time_period {
    start_date = formatdate("YYYY-MM-01'T'00:00:00Z", plantimestamp())
  }

  notification {
    enabled        = true
    threshold      = 80
    operator       = "GreaterThan"
    threshold_type = "Actual"
    contact_emails = var.alert_emails
  }
  notification {
    enabled        = true
    threshold      = 100
    operator       = "GreaterThan"
    threshold_type = "Actual"
    contact_emails = var.alert_emails
  }
  notification {
    enabled        = true
    threshold      = 100
    operator       = "GreaterThan"
    threshold_type = "Forecasted"
    contact_emails = var.alert_emails
  }

  lifecycle {
    ignore_changes = [time_period[0].start_date]
  }
}
