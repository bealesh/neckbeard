Everything is written and statically validated. Nothing was deployed, no cloud API was called, and nothing is committed (you didn't ask for a commit, so the files are left untracked for you to review).

**Neckbeard was not usable for this.** Its catalog only models containerized HTTP/worker/cron services with Postgres, Redis, object storage and secrets. Running its analyze step on a scratch copy stopped with "no containerized service found", so I wrote the configuration directly.

**What's in the directory now**

- `versions.tf`, `providers.tf`: OpenTofu 1.6+ with AWS provider 6.x, plus a `us_east_1` provider alias because CloudFront only accepts certificates from that region.
- `s3.tf`: private origin bucket with public access blocked, versioning, SSE-S3, a 30-day noncurrent-version expiry, and a bucket policy that only trusts this distribution and denies non-TLS access.
- `acm.tf`: DNS-validated certificate for www.example.com with validation records written into the hosted zone.
- `cloudfront.tf`: distribution using Origin Access Control, HTTPS redirect, TLS 1.2+, HTTP/2 and 3, the managed CachingOptimized and SecurityHeaders policies, and a small CloudFront Function that rewrites directory requests to index.html.
- `route53.tf`: A and AAAA alias records for www.example.com in the existing example.com zone.
- `github_oidc.tf`: the GitHub OIDC provider and a deploy role whose trust policy is bound to one repo and the main branch. Its permissions are limited to list, get, put and delete on the bucket and create-invalidation on the distribution.
- `.github/workflows/deploy.yml`: on push to main touching the site directory, assumes the role via OIDC, syncs non-HTML assets with a one-year immutable cache, syncs HTML with no-cache headers, then invalidates `/*` and waits for completion. Uses a `production` environment and a concurrency group.
- `outputs.tf`, `terraform.tfvars.example`, `backend.hcl.example`, `.gitignore`, `README.md` with the rollout steps.

**Assumptions I made**

- The public hosted zone for example.com already exists in Route 53 in this account.
- Only www.example.com is served. The apex domain is untouched.
- The bucket name defaults to the domain name.
- The account has no GitHub OIDC provider yet. A variable flips to a lookup if it does.
- State is local until you enable the commented S3 backend.
- Access logging, WAF, origin failover and geo restriction are deliberately omitted, each recorded as an inline checkov skip with its reason.

**Validation results**

| Check | Result |
| --- | --- |
| tofu fmt -check | clean |
| tofu init -backend=false and tofu validate | valid |
| checkov (terraform + github_actions) | 78 passed, 0 failed, 4 justified skips |
| actionlint | clean |

**What you need to do next**

1. Copy the tfvars example and set `github_repository` to this repo's owner/name. The trust policy binds to it exactly.
2. Run plan and apply with real AWS credentials. Certificate validation and distribution creation take several minutes.
3. Copy the three outputs into GitHub repository variables named `AWS_DEPLOY_ROLE_ARN`, `S3_BUCKET_NAME` and `CLOUDFRONT_DISTRIBUTION_ID`, and create a `production` environment in the repo.
4. Push to main.