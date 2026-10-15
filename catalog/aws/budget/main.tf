# Monthly cost budget for one environment, scoped by the provider's default
# tags (neckbeard-app / neckbeard-env). Budgets NOTIFY; they do not cap spend
# (DESIGN §10.2). The only hard ceiling neckbeard offers is enforced before
# anything exists: finops.monthly_ceiling refuses scaffold when the estimate
# exceeds it.
#
# Prerequisite the operator must do once per account: activate the
# user-defined cost allocation tags `neckbeard-app` and `neckbeard-env` in
# Billing → Cost allocation tags. Until they are active the tag filter below
# matches nothing and this budget never fires (docs/bootstrap.md says so).

resource "aws_budgets_budget" "this" {
  name         = "${var.name_prefix}-monthly"
  budget_type  = "COST"
  limit_amount = tostring(var.monthly_amount)
  limit_unit   = "USD" # AWS Budgets are denominated in USD regardless of finops.currency
  time_unit    = "MONTHLY"

  cost_filter {
    name   = "TagKeyValue"
    values = [format("user:neckbeard-env$%s", var.environment)]
  }
  cost_filter {
    name   = "TagKeyValue"
    values = [format("user:neckbeard-app$%s", var.app)]
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = var.alert_emails
  }
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = var.alert_emails
  }
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = var.alert_emails
  }
}
