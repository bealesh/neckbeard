All done and validated. Nothing was deployed and no cloud API was called.

**What I produced** in the repo root:

- **Terraform/OpenTofu** split across `versions.tf`, `providers.tf`, `variables.tf`, `locals.tf`, `s3.tf`, `cloudfront.tf`, `dns.tf`, `github-oidc.tf`, `outputs.tf`, plus `cloudfront/directory-index.js`, `terraform.tfvars.example`, `.gitignore` and the provider lock file.
- **Private S3 origin bucket** with public access blocked, versioning, SSE, a 30-day lifecycle for old versions, and a policy that only lets CloudFront read it through Origin Access Control.
- **CloudFront distribution** with HTTP/2 and HTTP/3, IPv6, managed caching and security-header policies, redirect-to-HTTPS, TLS 1.2 minimum, and a small edge function that rewrites directory URLs to their index page.
- **ACM certificate** issued in us-east-1 with Route 53 DNS validation, and A plus AAAA alias records for www.example.com.
- **GitHub OIDC deploy role** whose trust policy only accepts tokens from pushes to the main branch of one repository, with permissions limited to objects in this bucket and invalidations on this distribution.
- **Workflow** at `.github/workflows/deploy.yml` that runs on pushes to main touching the site, assumes the role via OIDC, syncs the site with deletes and sensible cache headers, then creates and waits on a full invalidation.
- **README.md** with an architecture sketch, file map, assumptions, and bootstrap steps.

**Verification**: `tofu fmt`, `tofu init` and `tofu validate` pass, and `actionlint` reports no issues. Checkov passes 51 checks and flags four optional hardening items on the distribution: access logging, WAF, origin failover, and geo restriction. I left those out as judgment calls for a marketing site and listed logging and WAF as follow-ups in the README.

**Assumptions I made**:

- The public hosted zone for example.com already exists and is looked up, not created. Only the www record is managed, not the apex.
- The GitHub repository is unknown here (the repo has no remote), so `github_repository` is a required variable with no default. Set it in a local tfvars file before planning.
- The account has no existing GitHub OIDC provider. If it does, flip `create_github_oidc_provider` to false.
- Bucket name equals the hostname. State stays local until you fill in the commented S3 backend stub.
- The workflow uses a GitHub environment named `production` and reads the role ARN, bucket, and distribution ID from repository variables, which the outputs supply.

**Next steps for you**: fill in the tfvars, apply once with your own credentials, copy the three outputs into GitHub repository variables, and push to main. Nothing is committed yet. The neckbeard plugin was not used because its catalog only models container workloads and cannot express an S3/CloudFront static site.