# Monthly billing budget scoped to this environment's project. Budgets NOTIFY;
# they do not cap spend (DESIGN §10.2). The only hard ceiling neckbeard offers
# is finops.monthly_ceiling, enforced before anything exists.
#
# Prerequisites the operator must have once per billing account: the Cloud
# Billing Budget API enabled and billing.budgets.create on the billing account
# (Billing Account Costs Manager or Administrator). Project-level roles are
# not enough; docs/bootstrap.md says so.

data "google_project" "this" {}

resource "google_monitoring_notification_channel" "email" {
  for_each     = toset(var.alert_emails)
  display_name = "${var.name_prefix} budget: ${each.value}"
  type         = "email"
  labels = {
    email_address = each.value
  }
}

resource "google_billing_budget" "this" {
  billing_account = var.billing_account
  display_name    = "${var.name_prefix}-monthly"

  budget_filter {
    projects = ["projects/${data.google_project.this.number}"]
  }

  amount {
    specified_amount {
      currency_code = var.currency
      units         = tostring(floor(var.monthly_amount)) # whole units; fractions are dropped
    }
  }

  threshold_rules {
    threshold_percent = 0.8
  }
  threshold_rules {
    threshold_percent = 1.0
  }
  threshold_rules {
    threshold_percent = 1.0
    spend_basis       = "FORECASTED_SPEND"
  }

  all_updates_rule {
    monitoring_notification_channels = [for c in google_monitoring_notification_channel.email : c.id]
    disable_default_iam_recipients   = false # billing admins keep their default emails too
  }
}
