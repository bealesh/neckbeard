Done. Everything is written to the repo root and validated locally. Nothing was applied, and no cloud API was called. Files are uncommitted since you did not ask for a commit.

**What I produced**

- **OpenTofu root module** in this directory, split by concern: `s3.tf`, `cloudfront.tf`, `acm.tf`, `dns.tf`, `iam_github.tf`, plus `variables.tf`, `locals.tf`, `outputs.tf`, `providers.tf` and `versions.tf`. The AWS provider is pinned to the 5.x line and the lock file is included.
- **S3 origin bucket** named after the hostname. It is private, versioned with 30-day expiry of old versions, SSE-S3 encrypted, public access blocked, and readable only by this CloudFront distribution through Origin Access Control. A deny-insecure-transport statement is included.
- **CloudFront distribution** with HTTPS redirect, TLS 1.2 minimum, HTTP/2 and 3, IPv6, compression, the AWS managed CachingOptimized and SecurityHeaders policies, and 404 and 403 mapped to a 404 page. A small CloudFront Function in `functions/index-rewrite.js` maps directory URLs to their index file.
- **ACM certificate** in us-east-1 via an aliased provider, DNS-validated through the existing hosted zone, with the distribution depending on the validation resource.
- **Route 53** A and AAAA alias records for www.example.com in the looked-up example.com zone.
- **OIDC deploy role** with trust pinned to one repository and the main branch. Its policy allows only list, get, put and delete on the site bucket, plus invalidations on this one distribution.
- **GitHub Actions workflow** at `.github/workflows/deploy.yml`. On every push to main it assumes the role with OIDC, syncs the site in two passes so HTML revalidates while other assets cache for a week, deletes removed files, invalidates `/*`, and waits for the invalidation to complete. No stored keys. It uses repository variables for the role ARN, bucket and distribution ID.
- **Supporting files**: a README with setup steps and assumptions, `terraform.tfvars.example`, a `.gitignore`, and a new `site/404.html` for the error page.

**Assumptions I made**

- The public hosted zone for example.com already exists in the account. It is looked up, not created.
- Only www.example.com is served. There is no apex redirect.
- Everything lives in us-east-1. Only the certificate is forced there.
- The account has no GitHub OIDC provider yet. A flag switches to reusing an existing one.
- State is local for the first human-run apply. A commented S3 backend block is ready in `versions.tf`.
- The workflow targets a GitHub environment named production, which you can create or remove.

**Verification results**

| Check | Result |
| --- | --- |
| tofu fmt | clean |
| tofu init and validate | pass |
| actionlint | pass |
| checkov | 51 pass, 4 fail |

The four checkov failures are intentional omissions for a public marketing site: access logging, WAF, origin failover and geo restriction. They are documented in the README rather than suppressed.

**One thing to know about the neckbeard plugin.** It is loaded in this session for infrastructure requests, but its catalog only models containerised apps. Its analyzer refused this repo because there is no Dockerfile, and it has no static-site, CDN, certificate or DNS modules. So the Terraform here is hand-authored and is outside neckbeard's ownership contract, cost estimates and validation gates. The README states this.

**Next steps for you**: copy the tfvars example and set your repository as OWNER/NAME, run the apply once with your own credentials, then set the three repository variables from the outputs. Pushes to main will deploy from then on.