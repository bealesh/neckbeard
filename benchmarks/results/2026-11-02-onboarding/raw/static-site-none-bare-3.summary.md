Everything is written and validated. OpenTofu reports the configuration valid, `tofu fmt` is clean, the workflow parses as YAML and `actionlint` passes with no findings. Nothing was deployed and no AWS API was called. The only network use was `tofu init -backend=false` downloading the AWS provider from the OpenTofu registry, which also created `.terraform.lock.hcl` (worth committing). Nothing has been committed.

**What I produced**

- `s3.tf`: private, versioned, encrypted bucket named from the domain plus account ID. Bucket policy grants read only to this CloudFront distribution via Origin Access Control and denies non-TLS access. Old object versions expire after 30 days.
- `acm.tf`: DNS-validated certificate for www.example.com issued in us-east-1 through an aliased provider, with validation records created in Route 53.
- `cloudfront.tf` and `functions/index-rewrite.js`: distribution with OAC, HTTPS redirect, HTTP/2 and HTTP/3, the AWS managed CachingOptimized and SecurityHeaders policies, a viewer-request function that maps directory URLs to `index.html`, and 403/404 mapped to a 404 page.
- `route53.tf`: A and AAAA alias records for www.example.com.
- `iam.tf`: GitHub OIDC identity provider plus a deploy role whose trust policy accepts only the subject for pushes to `main` of one repo. Its permissions are limited to listing and writing the bucket and invalidating that one distribution.
- `.github/workflows/deploy-site.yml`: on every push to `main` or manual dispatch, assumes the role with `aws-actions/configure-aws-credentials` using the OIDC token, syncs `./site` in two passes (assets cached one day, HTML must-revalidate, deletions propagated), creates a `/*` invalidation and waits for it. A concurrency group prevents overlapping deploys. It reads the role ARN, bucket and distribution ID from repository variables.
- `variables.tf`, `locals.tf`, `data.tf`, `providers.tf`, `versions.tf`, `backend.tf`, `outputs.tf`, `terraform.tfvars.example`, `.gitignore`, `README.md`.

**Assumptions I made**

- A public Route 53 hosted zone for example.com already exists and is looked up, not created.
- The account has no GitHub OIDC provider yet. Set `create_github_oidc_provider = false` if it does.
- Only www.example.com is served. No apex redirect was requested, so none is configured.
- State is local. A commented S3 backend block is in `backend.tf`.
- Bucket, IAM and the default provider use us-east-1. ACM is pinned to us-east-1 regardless.
- Missing pages serve `/404.html`. The site currently has no such file, so visitors would see a bare 404 until one is added.

**To go live** you would fill in `github_org` and `github_repo` in `terraform.tfvars`, run `tofu apply`, then copy three outputs into GitHub repository variables as shown in the README. The workflow does nothing until those variables exist and fails fast with a clear error if they are missing.