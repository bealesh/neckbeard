# Bootstrap (DESIGN §10.3): the one root a human applies with elevated
# credentials, once per environment. Creates, in order of importance:
#   1. the OpenTofu state backend (versioned, encrypted, locked) every later
#      apply depends on,
#   2. the CI OIDC trust — federation only, no static cloud keys ever (§10.1),
#   3. the per-env plan (read) and apply (write) roles the pipelines assume.
# This root itself keeps LOCAL state (the chicken has no egg yet); the runbook
# says so loudly and treats the bootstrap state file as an operator artifact.

data "aws_caller_identity" "current" {}

# --- 1. state backend ---

resource "aws_s3_bucket" "tfstate" {
  bucket = "${var.name_prefix}-tfstate"
}

resource "aws_s3_bucket_versioning" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id
  rule {
    id     = "housekeeping"
    status = "Enabled"
    filter {}
    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }
}

resource "aws_s3_bucket_public_access_block" "tfstate" {
  bucket                  = aws_s3_bucket.tfstate.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# --- 2. OIDC federation ---

locals {
  github = var.vcs == "github"
  issuer = local.github ? "token.actions.githubusercontent.com" : "gitlab.com"
  # Both jobs bind an environment, so GitHub subjects take the environment form;
  # GitLab subjects carry the project path (branch scoping enforced by the
  # protected-branch gate, §11.3).
  subjects = local.github ? ["repo:${var.repo}:environment:${var.environment}"] : ["project_path:${var.repo}:*"]
  audience = local.github ? "sts.amazonaws.com" : "https://gitlab.com"
}

resource "aws_iam_openid_connect_provider" "ci" {
  url             = "https://${local.issuer}"
  client_id_list  = [local.audience]
  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1"] # informational: AWS validates GitHub/GitLab against trusted CAs
}

data "aws_iam_policy_document" "ci_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.ci.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.issuer}:aud"
      values   = [local.audience]
    }
    condition {
      test     = "StringLike"
      variable = "${local.issuer}:sub"
      values   = local.subjects
    }
  }
}

# --- 3. plan (read) and apply (write) roles ---
# Trust-boundary note, stated rather than implied: both roles trust the same
# repository subject; which role a job assumes is workflow logic, and prd is
# additionally gated by environment protection / the protected branch (§11.3).
# Fork PRs never receive an OIDC token.

data "aws_iam_policy_document" "state_access" {
  statement {
    actions   = ["s3:ListBucket", "s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = [aws_s3_bucket.tfstate.arn, "${aws_s3_bucket.tfstate.arn}/*"]
  }
}

resource "aws_iam_role" "plan" {
  name               = "${var.name_prefix}-ci-plan"
  assume_role_policy = data.aws_iam_policy_document.ci_assume.json
}

resource "aws_iam_role_policy_attachment" "plan_readonly" {
  role       = aws_iam_role.plan.name
  policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}

resource "aws_iam_role_policy" "plan_state" {
  name   = "tfstate-access"
  role   = aws_iam_role.plan.id
  policy = data.aws_iam_policy_document.state_access.json
}

resource "aws_iam_role" "apply" {
  name               = "${var.name_prefix}-ci-apply"
  assume_role_policy = data.aws_iam_policy_document.ci_assume.json
}

# PowerUser + IAM scoped to the app's name prefix. An exhaustively least-privilege
# apply policy is roadmap work the release harness will shape; the honest state of
# this one: broad on resources, narrow on IAM principals it may create.
resource "aws_iam_role_policy_attachment" "apply_poweruser" {
  role       = aws_iam_role.apply.name
  policy_arn = "arn:aws:iam::aws:policy/PowerUserAccess"
}

data "aws_iam_policy_document" "apply_iam" {
  statement {
    actions = ["iam:*"]
    resources = [
      "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/${var.name_prefix}-*",
      "arn:aws:iam::${data.aws_caller_identity.current.account_id}:policy/${var.name_prefix}-*",
      "arn:aws:iam::${data.aws_caller_identity.current.account_id}:instance-profile/${var.name_prefix}-*",
    ]
  }
  statement {
    actions   = ["iam:GetOpenIDConnectProvider", "iam:GetRole", "iam:ListAttachedRolePolicies", "iam:ListRolePolicies"]
    resources = ["*"]
  }
  statement {
    actions   = ["iam:CreateServiceLinkedRole"]
    resources = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/aws-service-role/*"]
  }
}

resource "aws_iam_role_policy" "apply_iam" {
  name   = "iam-scoped-to-prefix"
  role   = aws_iam_role.apply.id
  policy = data.aws_iam_policy_document.apply_iam.json
}

resource "aws_iam_role_policy" "apply_state" {
  name   = "tfstate-access"
  role   = aws_iam_role.apply.id
  policy = data.aws_iam_policy_document.state_access.json
}
