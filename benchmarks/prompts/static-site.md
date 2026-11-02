Set up hosting for the static marketing site in ./site on AWS: an S3 bucket behind CloudFront with an ACM certificate and a Route 53 record for www.example.com, as OpenTofu/Terraform in this directory, plus a GitHub Actions workflow that syncs ./site to the bucket and invalidates the CloudFront cache on every push to main, authenticating with OIDC rather than stored keys.

Do not deploy anything and do not call any cloud API. Do not ask me questions: make reasonable assumptions, state them, and proceed to completion. Summarize what you produced when done.
