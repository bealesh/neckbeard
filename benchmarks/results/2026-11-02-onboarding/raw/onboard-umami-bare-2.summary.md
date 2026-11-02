Everything is generated, formatted, validated with OpenTofu 1.12.6 (provider downloads only, no cloud calls), and the workflows pass actionlint. Nothing was deployed and nothing is committed; the new files are untracked in `infra/` and `.github/workflows/aws-*.yml`.

## What was generated

**OpenTofu, under `infra/`** (about 2,400 lines including docs)

- `bootstrap/`: one-time, human-applied stack with local state. Creates the versioned, encrypted S3 state bucket (S3 native locking, no DynamoDB), the GitHub OIDC provider, a read-only plan role for pull requests, and a deploy role trusted only by pushes to `main` and jobs bound to the `dev`, `stg`, `prd` GitHub environments.
- `global/`: one ECR repository with immutable tags and a lifecycle policy, the Route 53 hosted zone, and an AWS Budget with email alerts.
- `environment/`: a single root module applied three times with `envs/dev|stg|prd.tfvars`. It wires three modules: `network` (VPC, 2 AZs, public subnets for ALB and tasks, private subnets for the database, no NAT), `database` (single RDS PostgreSQL 17, generated password, storage autoscaling, backups), and `app` (ECS Fargate service, ALB with ACM certificate and HTTP to HTTPS redirect, Route 53 alias, CloudWatch logs, CPU autoscaling, circuit-breaker rollback). The root also holds the three security groups, one Secrets Manager secret per environment with `DATABASE_URL`, `APP_SECRET`, `TWO_FACTOR_ENCRYPTION_KEY`, and optional alarms routed to SNS email.
- Environments differ only in tfvars: dev and stg run one Fargate Spot task on `db.t4g.micro`; prd runs two on-demand tasks across AZs, scales to four, uses `db.t4g.small`, 7-day backups, deletion protection, and alarms.

**GitHub Actions**, alongside the untouched upstream workflows

- `aws-pr.yml`: builds the image without pushing, runs fmt and validate, then plans global, dev, stg, and prd and upserts each plan as a PR comment.
- `aws-deploy.yml`: on `main`, builds and pushes the image tagged with the commit SHA, applies global, then deploys dev, stg, prd in order. Manual runs accept an existing image tag for rollback and can target one environment.
- `aws-deploy-env.yml`: the reusable plan, apply, and heartbeat smoke test per environment. Only two repository variables are needed, the account ID and region. There are no repository secrets at all.

**`infra/COSTS.md`** with per-line unit prices and the levers.

## Where a reviewer should start

`infra/README.md` gives a numbered reading order. The short version is README, then `COSTS.md`, then `infra/environment/main.tf` and its siblings, then diff `envs/prd.tfvars` against `envs/dev.tfvars`, then `infra/bootstrap/main.tf` for the trust boundary.

## Estimated monthly cost

| dev | stg | prd | shared | total |
|---:|---:|---:|---:|---:|
| $46 | $48 | $101 | $1 | about $196 |

The biggest fixed items are the three load balancers and public IPv4 charges, not compute. `COSTS.md` lists the levers: destroying dev when idle or running prd as a single task each save roughly $20 to $46, and a stripped-down floor is about $80.

## Assumptions and trade-offs to confirm

- One AWS account, us-east-1, a domain you will delegate to Route 53. `example.com` and `your-github-user/umami` are placeholders in the tfvars and must be replaced.
- PostgreSQL only. Redis, ClickHouse, and Kafka are optional in Umami and not provisioned.
- No NAT gateway. Tasks get public IPs but only accept traffic from the load balancer. The database has no internet route.
- TLS to RDS is encrypted but the server certificate is not verified, because the image lacks the RDS CA bundle. The upgrade path is documented in the database module.
- Generated secrets live in OpenTofu state as well as Secrets Manager, and the PR plan role can read them. The deploy role is deliberately broad for a solo developer.
- Images are `linux/amd64` only. Switching to ARM saves about $7 a month in prd but needs an arm64 build.
- Unit prices are quoted from memory and should be checked in the AWS pricing calculator before you rely on the totals.

Next step on your side is replacing the placeholders, running bootstrap once with admin credentials, and setting required reviewers on the `prd` GitHub environment.