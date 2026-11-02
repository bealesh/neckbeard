Done. Everything is generated, statically validated, and left uncommitted in the working tree for review. Nothing was deployed and no cloud API was called.

## What I generated

- **Two hand-written inputs** that drive everything: `app-profile.yaml` (the app's three process roles, port, health path, Postgres need, every assumption paired with its answer and file:line evidence) and `neckbeard.yaml` (AWS, us-east-1, ECS Fargate, solo tier, budget, ceiling, one override).
- **OpenTofu for dev, stg, prd** under `infra/envs/<env>/` plus a human-applied `infra/bootstrap/<env>/` root per environment. Each environment gets its own VPC, ALB with TLS wiring, Fargate cluster running `web`, `worker` and a daily `report` task, RDS PostgreSQL 16, a Secrets Manager container for DATABASE_URL, ECR, and an AWS Budget. Bootstrap creates the versioned encrypted S3 state bucket, the GitHub OIDC provider, and read-only plan plus main-only apply roles.
- **Three GitHub Actions workflows**: CI tests then builds, Trivy-scans and pushes the image on main. Infra runs checkov and plans on PRs, applies on main behind GitHub environments. Release deploys a committed image digest by manual dispatch. All cloud access is OIDC; no static keys anywhere.
- **Cost report** at `costs/estimate.md`, and a reviewer README at `README.md`.

## Where to start

Read `README.md`, then `app-profile.yaml`, `neckbeard.yaml`, `costs/estimate.md`, and `infra/envs/prd/main.tf`. The README gives the full reading order and the assumptions list.

## Estimated monthly cost

| | baseline | expected | high |
|---|---|---|---|
| per environment | 123.00 | 129.04 | 153.11 |
| all three, USD/month | 369.00 | 387.12 | 459.33 |

List prices, no free tier or tax. The biggest fixed items per environment are the NAT gateway, the ALB, the two always-on tasks, Container Insights, and the micro RDS instance. I replaced the preset's four VPC interface endpoints with one NAT gateway via a catalog-supported override, which saved about $76 a month across the three environments. The cheapest further lever is destroying stg between release candidates.

## Assumptions you should check

- Repository slug `stockitbetter/bellwether` and the budget email `budget-alerts@example.com` are placeholders. OIDC trust binds to the slug exactly.
- Solo tier: single-AZ database, no WAF, 7-day backups in prd, no on-call.
- The report job runs daily at 06:00 UTC. The repo specifies no cadence.

## Things to know

- **The committed Dockerfile is truncated** on three lines and will not build. I did not edit application code. Restore the binary path before the first CI build.
- **Application deploys are not fully automatic on main.** Infrastructure applies on main, but app releases pin a reviewed digest and are triggered per environment by hand. This is the toolchain's design and I could not change it without hand-editing generated pipelines, which its ownership contract forbids.
- **HTTPS needs an ACM certificate per environment** supplied through a small tfvars file. The release runner refuses to deploy without an HTTPS health URL, so this is a prerequisite, not a later step.
- Validation passed syntax, schema, checkov and actionlint on all six roots. Authenticated plans and deployment were not exercised because there are no credentials here.

Next step for the operator is `docs/bootstrap.md`, once per environment with elevated credentials.