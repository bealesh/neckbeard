Done. Everything is generated and validated locally; nothing was deployed and no cloud API was called.

## What was generated

**OpenTofu in `infra/`** (all five roots pass `tofu validate` against the real AWS 6.71 and random 3.9 provider schemas; lock files committed):

- `bootstrap/` is the one stack a human applies: the versioned, encrypted S3 state bucket with S3 native locking, the GitHub OIDC provider, and two IAM roles. A read-only role is trusted for pull-request plans and an admin role for `main` and the four GitHub environments.
- `global/` holds the ECR repository with immutable tags and scanning, the Route 53 hosted zone, and an AWS Budget that emails at 80% actual and 100% forecast.
- `envs/dev`, `envs/stg`, `envs/prd` are thin roots. Each calls `modules/environment` with its knobs, so a diff of two `main.tf` files shows exactly how the environments differ.
- `modules/network`, `database`, `ingress`, `app`, `environment` build a VPC per env, RDS PostgreSQL 17 in private subnets, an ALB with an ACM certificate and HTTP-to-HTTPS redirect, and ECS Fargate on arm64 running the three app modes: a `serve` service behind the ALB, a `work` service with no load balancer, and `report` as a daily EventBridge Scheduler task. Dev and stg scale both services to zero outside weekday office hours.
- Secrets never touch state. The DB password is an ephemeral value written through write-only arguments to both RDS and a Secrets Manager secret holding the full DSN, which ECS injects as `DATABASE_URL`. Bumping one version number rotates everything and rolls the services.

**GitHub Actions in `.github/workflows/`** (lint clean with actionlint): `pr.yml` runs Go checks, a no-push image build, fmt, and a plan matrix over all four stacks posted as PR comments. `deploy.yml` applies global, builds and pushes `sha-<commit>` to ECR, then rolls dev, stg, and prd through GitHub environments. All AWS access uses OIDC with no stored keys. PR plans reuse the deployed image tag from an SSM parameter so they show infrastructure diffs only.

**Docs**: `COSTS.md` with the estimate and assumptions, `infra/README.md` as the reviewer entry point with setup runbook and security notes, and a short root `README.md`.

## Where a reviewer should start

`infra/README.md` gives a six-step reading order. Start with `envs/prd/main.tf`, then `modules/environment/main.tf`, `modules/app/main.tf`, `modules/database/main.tf`, `bootstrap/main.tf` for the trust policies, and finally `deploy.yml`.

## Estimated monthly cost

| Environment | As configured |
| --- | ---: |
| dev (office hours) | $47 |
| stg (office hours) | $47 |
| prd (24x7, 2 web tasks) | $86 |
| shared | $1 |
| **Total** | **about $181** |

Running everything 24x7 would be about $209. The three ALBs and three databases are two thirds of the bill.

## Assumptions worth checking

- Single AWS account, region `us-east-1`, a domain you can delegate to Route 53. Placeholder values (`example.com`, `you@example.com`) sit in committed `terraform.tfvars` files and must be edited first.
- Tasks run in public subnets with a public IP and no NAT gateway to save about $30 per environment. A single flag moves them private.
- Prd RDS is single-AZ with 7-day backups and deletion protection. Multi-AZ is one variable and about $26 more.
- The deploy role is AdministratorAccess, constrained by its trust policy rather than its permission policy.

## One thing I changed outside the ask

The committed Dockerfile was broken: the binary path was missing from the `go build -o`, `COPY`, and `ENTRYPOINT` lines, so the image workflow would have failed. I rewrote it to cross-compile for the target platform, then built it for arm64 and smoke-tested `serve` and `report` in a container, both of which worked.

Nothing is committed; the working tree has the new files plus the modified Dockerfile for you to review.