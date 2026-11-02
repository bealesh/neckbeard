Done. Everything is generated under `infra/` and `.github/workflows/`, left uncommitted on `main` for you to review. Nothing was deployed and no cloud API was called.

**What was generated**

- `infra/app/` is one root module that becomes an environment when paired with `infra/envs/<env>.tfvars` and `<env>.s3.tfbackend`. It wires three modules: `network` (two-AZ VPC, no NAT), `database` (RDS PostgreSQL 17, forced SSL, two Secrets Manager secrets) and `service` (ECS Fargate, ALB with ACM certificate, Route 53 alias, HTTP to HTTPS redirect, rolling deploys with circuit-breaker rollback).
- `infra/bootstrap/` is the one-time account stack: versioned and encrypted S3 state bucket using the native lock file, GitHub OIDC provider, one read-only plan role, one ECR push role, three per-environment deploy roles, the ECR repository with immutable tags, the hosted zone, and a free AWS Budgets alert.
- `.github/workflows/ci.yml` builds the image without pushing, runs format and validate, and posts a sticky plan comment per environment on every PR. `deploy.yml` pushes the image tagged with the git SHA on `main`, then calls `_apply.yml` for dev, stg and prd in order, each under a GitHub environment and a `/healthz` smoke test. No stored cloud keys anywhere.
- `infra/COST.md` holds the estimate with unit prices, per-line calculations and levers.
- `infra/README.md` holds the architecture, assumptions, first-time setup, and the security trade-offs.

**Verification**

- Both root modules pass `tofu init -backend=false` and `tofu validate` on OpenTofu 1.12.6 with AWS provider 6.71. Lock files are committed for Linux and macOS.
- `tofu fmt -check` is clean. `actionlint` passes on all three workflows. The Dockerfile builds locally.

**Where a reviewer should start**

1. `infra/app/main.tf`, the whole environment on one page.
2. `infra/envs/*.tfvars`, the only differences between environments.
3. `infra/modules/database/main.tf`, the ephemeral password and write-only secret pattern.
4. `infra/bootstrap/main.tf`, the OIDC trust policies and roles.
5. The three workflows.

**Key assumptions** (all listed in the README)

- Single AWS account and region, with environments isolated by VPC, state file and IAM role.
- The app needs only Postgres and makes no outbound calls, so tasks sit in public subnets behind a locked-down security group instead of paying for a NAT gateway.
- You own a domain and will delegate it to Route 53. The tfvars use a placeholder domain and a placeholder account id in the backend files.
- Production RDS is single-AZ with 7-day point-in-time recovery. Multi-AZ is one variable.
- Deploy roles carry AdministratorAccess, bounded by trust policy and GitHub environment protection.
- Database secrets never land in state, which means the PR plan role must be able to read them. The README explains why and the alternative.

**Estimated monthly cost** (us-east-1 list prices, 24x7, no free tier)

| Environment | USD / month |
|---|---:|
| dev | ~46 |
| stg | ~46 |
| prd | ~80 |
| shared | ~1 |
| Total | ~173 |

The ALB and its two public addresses are the largest line in every environment. The README's first-time setup section covers the bootstrap apply, delegating the domain, and the two GitHub repository variables and three GitHub environments you need to create before CI can run.