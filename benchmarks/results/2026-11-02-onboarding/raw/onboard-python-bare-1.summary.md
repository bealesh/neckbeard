Everything is generated, formatted, and validated locally. Nothing was deployed and no cloud API was called. Files are left uncommitted in the working tree for you to review.

**What was generated**

- `infra/bootstrap/`: one-time account foundations. S3 state bucket with native lockfile locking, ECR repository with immutable tags, Route 53 hosted zone, GitHub OIDC provider, a read-only plan role and a scoped deploy role, and an AWS Budgets alarm.
- `infra/modules/`: four modules. `network` (VPC, two public and two private subnets, three security groups, no NAT), `database` (RDS PostgreSQL 16 plus the `DATABASE_URL` secret in Secrets Manager), `service` (ECS Fargate ARM64, ALB with ACM certificate, HTTP to HTTPS redirect, Route 53 alias), and `monitoring` (four CloudWatch alarms to an email address).
- `infra/envs/dev|stg|prd/`: root stacks with separate state files. Their code is byte-identical and CI enforces that, so only `terraform.tfvars` and the backend key differ.
- `.github/workflows/`: `ci.yml` builds the image and posts three plans on pull requests. `deploy.yml` pushes the image on merge to main and applies dev, then stg, then prd through the reusable `deploy-env.yml`. All AWS access is via OIDC role assumption, with no stored keys.
- `infra/COSTS.md` and `infra/README.md`, plus `.gitignore`, `dependabot.yml`, and committed provider lock files for Linux and macOS.

**Verification done here**: `tofu validate` passes for all four roots on AWS provider 6.71, `tofu fmt` is clean, and actionlint passes on the workflows. This required downloading providers from the public registry only.

**Where a reviewer should start**: `infra/README.md` has a numbered reading order. The short version is `envs/prd/terraform.tfvars`, then `envs/prd/main.tf`, then the modules in order, then the trust policies in `bootstrap/main.tf`.

**Estimated monthly cost** (us-east-1, list prices, low traffic):

| Environment | USD/month |
|---|---:|
| dev | ~51 |
| stg | ~51 |
| prd | ~66 |
| shared | ~1 |
| Total | ~169 |

The ALBs and their IPv4 addresses are about 43% of that. Destroying dev when idle saves ~$51. Full line items and levers are in `infra/COSTS.md`.

**Assumptions I made**

- The database is PostgreSQL. The app only reads `DATABASE_URL`, so the URL is stored whole in Secrets Manager and the generated password therefore lives in encrypted state.
- No other datastore is needed. The code uses no cache, queue, or object storage.
- One AWS account, one region (us-east-1), placeholder project slug `myapp`, domain `example.com`, and account id `123456789012`. The README lists each placeholder and where it appears.
- Tasks get public IPs instead of a NAT gateway to stay on budget. The database has no internet route.
- prd runs two tasks but a single-AZ db.t4g.micro. Multi-AZ is a one-line toggle costing about $14 more.
- prd deploys should be gated by a required reviewer on the GitHub `prd` environment, which you configure in the GitHub UI.

**Next steps for you**: replace the placeholders, apply `bootstrap` once with your own admin credentials, delegate the domain's nameservers, set the `AWS_ACCOUNT_ID` repository variable and the three GitHub environments, then merge to main.