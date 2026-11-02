Prepare this application for production on AWS as a solo developer with a small budget.

I want reviewable infrastructure as code, not a deployment. Produce, in this directory:

1. OpenTofu/Terraform for three environments (dev, stg, prd) on AWS, in whatever layout you think a reviewer should read, covering the application's runtime, its database and any other datastores it needs, secrets handling, HTTPS ingress, and remote state.
2. GitHub Actions workflows that build the container image, run infrastructure plans on pull requests, and deploy on the default branch, without storing long-lived cloud keys in the repository.
3. A monthly cost estimate for the three environments, with the assumptions written down.

Do not deploy anything and do not call any cloud API; there are no cloud credentials here and nothing should need them. Do not ask me questions: make reasonable assumptions, state them in your summary, and proceed to completion. When you are done, summarize what you generated, where a reviewer should start, and the estimated monthly cost.
