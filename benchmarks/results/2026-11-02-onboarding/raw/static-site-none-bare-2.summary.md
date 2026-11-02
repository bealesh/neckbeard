Done. Nothing was deployed and no AWS API was called. The config passes `tofu validate` and the workflow passes `actionlint`. Files are left uncommitted on `main`.

**What I produced**

- **Terraform/OpenTofu in the repo root.** `s3.tf` creates a private, versioned, SSE-encrypted bucket with public access blocked and a policy that only allows this CloudFront distribution to read it. `cloudfront.tf` sets up the distribution with Origin Access Control, HTTPS redirect, HTTP/2 and 3, IPv6, and the AWS-managed caching and security-headers policies. A CloudFront Function in `functions/index-rewrite.js` maps directory URLs to their `index.html`.
- **Certificate and DNS.** `acm.tf` requests a DNS-validated certificate in us-east-1 via a second provider alias and creates the validation records. `route53.tf` adds A and AAAA alias records for www.example.com in the existing hosted zone.
- **OIDC deploy role.** `github_oidc.tf` creates the GitHub identity provider and a role whose trust policy only accepts tokens from your repo's `main` branch. Its permissions are limited to listing, putting, and deleting objects in the one bucket plus creating invalidations on the one distribution.
- **Workflow.** `.github/workflows/deploy.yml` runs on pushes to `main` that touch `site/` or the workflow. It assumes the role with `aws-actions/configure-aws-credentials`, syncs HTML with no-cache headers and other assets with a one-day cache, then invalidates `/*` and waits for completion.
- **Supporting files.** `README.md` documents the layout, assumptions, and setup steps. `terraform.tfvars.example`, `outputs.tf`, and `.gitignore` round it out.

**Assumptions I made**

- A public Route 53 hosted zone for example.com already exists. It is looked up, not created, and only the www host is configured. No apex redirect.
- The bucket lives in us-east-1 by default and is named `www-example-com-<account-id>` unless you override `bucket_name`.
- The account has no existing GitHub OIDC provider. If it does, set `create_github_oidc_provider = false` and the existing one is reused.
- State is local, with a commented S3 backend block ready in `versions.tf`.
- Role ARN, bucket name, and distribution ID are passed to the workflow as GitHub repository variables, since none are secrets.
- No WAF, access logging, or apex domain. Each is a small addition if wanted.

**Next steps for you**

1. Copy `terraform.tfvars.example` to `terraform.tfvars` and set `github_repository`.
2. Run `tofu init`, `tofu plan`, and `tofu apply`.
3. Create the three repository variables in GitHub from the Tofu outputs, plus a `production` environment.
4. Commit these files and push to `main` to trigger the first deploy.

One caveat: running `tofu validate` locally required downloading the AWS provider from the OpenTofu registry. That is a registry fetch, not a cloud API call, and I deleted the resulting `.terraform` directory afterward.