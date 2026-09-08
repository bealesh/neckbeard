output "state_bucket" {
  value = aws_s3_bucket.tfstate.bucket
}

output "backend_hcl" {
  description = "Contents for infra/envs/<env>/backend.hcl (the pipelines init with it)."
  value       = <<-EOT
    bucket       = "${aws_s3_bucket.tfstate.bucket}"
    key          = "envs/${var.environment}/terraform.tfstate"
    region       = "${var.region}"
    use_lockfile = true
  EOT
}

output "plan_role_arn" {
  value = aws_iam_role.plan.arn
}

output "apply_role_arn" {
  value = aws_iam_role.apply.arn
}

output "ci_variables" {
  description = "CI variables the generated pipelines expect (set via gh/glab, see docs/bootstrap.md)."
  value = {
    "NECKBEARD_AWS_PLAN_ROLE_${upper(var.environment)}"  = aws_iam_role.plan.arn
    "NECKBEARD_AWS_APPLY_ROLE_${upper(var.environment)}" = aws_iam_role.apply.arn
  }
}
