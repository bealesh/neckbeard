Everything is in place and verified offline. Nothing was deployed, no cloud API was called, and nothing was committed.

## What I generated

**OpenTofu (`infra/`, 36 files).** One module per environment with a file per concern, three thin environment roots, and a one-time bootstrap stack.

- `infra/modules/umami/`: VPC with two public and two private subnets and no NAT gateway, security groups chained internet to ALB to app to database, RDS PostgreSQL 17 with forced TLS, a Secrets Manager secret holding `DATABASE_URL`, `APP_SECRET` and the 2FA key, ACM certificate plus ALB with HTTP to HTTPS redirect, ECS Fargate service with rolling deploys and circuit-breaker rollback, scoped IAM roles, CPU autoscaling, and five alarms to an SNS topic.
- `infra/envs/{dev,stg,prd}/`: sizing only. Dev and stg run one Spot task and a db.t4g.micro. Prd runs two on-demand tasks scaling to four, a db.t4g.small, deletion protection, 7-day backups and alarms on.
- `infra/bootstrap/`: S3 state bucket with S3 lockfile locking (no DynamoDB), GitHub OIDC provider with a read-only plan role and a main-branch-only apply role, the ECR repository, per-environment SSM parameters that record the released image tag, and an AWS Budget alert.
- Provider lock files were generated for linux and macOS on both architectures.

**GitHub Actions (`.github/workflows/`).** `infra-plan.yml` runs fmt, validate and a read-only plan for all three environments on pull requests and posts a sticky PR comment. `image-build.yml` proves the Dockerfile builds on pull requests without pushing. `deploy.yml` with `deploy-env.yml` builds and pushes a `sha-` tagged image on push to main, then applies dev, stg and prd in sequence with a heartbeat smoke test between stages. Manual runs redeploy one environment with an existing tag, which is the rollback path. All AWS access is OIDC. A Dependabot config keeps actions, providers and the base image current.

**Docs.** `infra/README.md` covers architecture, reading order, assumptions, a trade-off table with how to undo each one, first-time setup and day-2 runbook. `infra/COSTS.md` has the estimate.

## Where a reviewer should start

1. `infra/envs/prd/main.tf` for the production shape in twenty lines.
2. `infra/modules/umami/variables.tf`, then the module files in the order the README lists.
3. `infra/bootstrap/github-oidc.tf` for what CI may do.
4. `.github/workflows/deploy.yml` for the release path.

## Estimated monthly cost

| Scope | USD/month |
|---|---:|
| dev | 47 |
| stg | 47 |
| prd | 98 |
| shared (ECR, Route 53, state) | 1 |
| **All three running continuously** | **≈ 192** |
| prd only, dev and stg destroyed between uses | ≈ 99 |

Load balancers and their public IPv4 addresses are the largest fixed cost at roughly 27 dollars per environment. The levers table in `COSTS.md` prices each alternative, including a shared ALB, ARM64 Fargate, a single prd task and Multi-AZ.

## Key assumptions

- Single AWS account in us-east-1, with an existing public Route 53 zone. Hostnames and the zone are placeholders in `infra/envs/*/terraform.tfvars`.
- Images build for linux/amd64 and run on x86 Fargate so the build never needs QEMU or an ARM runner.
- Promotion is automatic from main through dev, stg and prd with no manual gate, since GitHub environment protection rules are unavailable on free private repos. The README shows where to add one.
- Umami runs its own Prisma migrations at container start, so there is no migration job.
- Only PostgreSQL is provisioned. Redis and ClickHouse are cloud-mode options Umami does not need here.
- TLS to RDS is forced but the Amazon CA is not verified, because the image does not ship the RDS bundle. The README explains the upgrade to verify-full.

## Verification and limits

`tofu fmt -check` and `tofu validate` pass for all four stacks against AWS provider 6.x. `actionlint` passes for the four workflows. No plan was run, because that needs credentials and a real hosted zone. Unit prices in the cost sheet come from my knowledge and should be checked against the AWS Pricing Calculator. The upstream `cd.yml` still pushes to GHCR on version tags, which I left untouched.