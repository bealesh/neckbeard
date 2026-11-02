Everything is in place and validated. Nothing was deployed and no cloud API was called.

**What I produced** in the project directory, all untracked and uncommitted:

- `versions.tf`, `providers.tf`, `variables.tf`, `locals.tf`: OpenTofu config with AWS provider 6.x, a default region provider plus a us-east-1 alias for ACM, inputs, naming and tags.
- `s3.tf`: private bucket with public access blocked, SSE-S3, versioning with 30-day expiry of old versions, and a policy that lets only this CloudFront distribution read it.
- `acm.tf`: DNS-validated certificate for www.example.com with the validation records created in Route 53.
- `cloudfront.tf` and `functions/rewrite-index.js`: Origin Access Control, a viewer-request function that rewrites `/path/` to `/path/index.html`, and the distribution with HTTPS redirect, HTTP/2 and 3, compression, AWS managed caching and security-header policies, and an optional custom 404 page.
- `dns.tf`: A and AAAA alias records for www.example.com.
- `github_oidc.tf`: the GitHub OIDC identity provider and a deploy role whose trust policy only accepts tokens from pushes to `main` of one repository. Its permissions are limited to syncing the bucket and invalidating the distribution.
- `outputs.tf`, `terraform.tfvars.example`, `.gitignore`, `.terraform.lock.hcl`, `README.md` with setup steps.
- `.github/workflows/deploy.yml`: on every push to main, or manual dispatch, assumes the role via OIDC, syncs `./site` with delete, and creates and waits on a `/*` invalidation. HTML is uploaded with must-revalidate cache headers and other files with a one-hour max-age.

**Verification**: `tofu fmt`, `tofu init -backend=false` and `tofu validate` all pass. The workflow parses as YAML and `actionlint` reports no issues. The CloudFront function passes a Node syntax check.

**Assumptions I made**:

- A public Route 53 hosted zone for example.com already exists. The config looks it up and does not create it.
- Only www.example.com is served. The apex domain is out of scope, and the README describes what adding it would take.
- Everything lives in us-east-1 by default. The region is a variable.
- The bucket is named after the domain with dashes plus the account ID, to avoid dots in the name and global name collisions.
- State is local. A commented S3 backend block is in `versions.tf`.
- The account has no existing GitHub OIDC provider. If it does, set `create_github_oidc_provider = false` and the existing one is used.
- CloudFront price class is PriceClass_100, covering North America and Europe.
- The role ARN, bucket name and distribution ID are passed to the workflow as GitHub repository variables named `AWS_ROLE_ARN`, `S3_BUCKET` and `CLOUDFRONT_DISTRIBUTION_ID`, since none are secret.

**Next steps for you**: copy the tfvars example, set `github_repository` to your owner/name, run `tofu apply`, then create the three repository variables from the outputs and push to main.