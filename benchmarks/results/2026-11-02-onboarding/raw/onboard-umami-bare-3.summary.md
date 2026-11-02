Done. Everything is generated, validated offline, and left uncommitted in the working tree. No cloud API was called.

## What I generated

**Infrastructure (`infra/`, OpenTofu, AWS provider 6.x)**
- `bootstrap/`: one-time admin stack. S3 state bucket with native lockfile locking, GitHub OIDC provider, a read-only plan role and a deploy role scoped to `umami-*` IAM names, and a monthly cost budget.
- `shared/`: one ECR repository with immutable tags and a lifecycle policy.
- `modules/network`, `database`, `app`, `environment`: VPC without NAT, RDS PostgreSQL 17, ECS Fargate on Graviton behind an ALB with an ACM certificate and Route 53 record, Secrets Manager injection, CloudWatch logs and six alarms per environment, autoscaling.
- `environments/dev|stg|prd`: thin roots. Each is one module call where every per-environment difference is visible in a single file.
- `README.md` (architecture, assumptions, runbook), `COSTS.md`, `Makefile`, `backend.hcl`.

**GitHub Actions (`.github/`)**
- `aws-image.yml`: builds the Dockerfile on PRs, builds and pushes to ECR tagged with the git SHA when called by the deploy workflow.
- `aws-infra-plan.yml`: fmt, validate, and `tofu plan` for all stacks on PRs touching infra, posted as PR comments using the read-only role.
- `aws-deploy.yml` plus a composite action: on push to `main`, image → shared → dev → stg → prd, with prd gated by a GitHub Environment approval. All AWS access is OIDC; no keys stored anywhere.

**Validation performed**: `tofu fmt -check` and `tofu validate` pass for all five stacks against the real provider schemas, and `actionlint` passes on the three workflows. Nothing was applied.

## Where a reviewer should start

1. `infra/README.md` for the diagram and the assumptions table.
2. `infra/modules/environment/main.tf` for the complete security-group topology.
3. `infra/environments/prd/main.tf` for the sizing knobs.
4. `infra/modules/app/ecs.tf` for the task definition and the SSM-parameter deploy mechanism.
5. `.github/workflows/aws-deploy.yml` for how a commit reaches production.

## Estimated monthly cost

| Environment | Estimate |
|---|---|
| dev (Spot, scaled to zero nights and weekends) | about $42 |
| stg (Spot, always on) | about $47 |
| prd (2 on-demand tasks, db.t4g.small) | about $92 |
| Total | about $180, range $165 to $200 |

The ALBs are the biggest idle cost. Destroying stg between releases saves about $47 while it is down. Full assumptions and levers are in `infra/COSTS.md`.

## Key assumptions to check

- Single AWS account, region us-east-1, and an existing public Route 53 zone.
- Umami's default PostgreSQL mode only. No Redis, ClickHouse or Kafka.
- Tasks use public IPs instead of a NAT gateway, locked down by security group.
- RDS is single-AZ everywhere. Multi-AZ for prd is a one-line change.
- Generated secrets also live in the encrypted state file.
- The app connects to RDS with TLS but without certificate verification, because the upstream image lacks the RDS CA bundle.
- The image builds on GitHub's `ubuntu-24.04-arm` runner. If that runner is unavailable on your plan, switch both the workflow and `cpu_architecture` to x86.

Next steps are yours. Fill in `backend.hcl` and the tfvars domain names, apply bootstrap locally, set the three GitHub repository variables, create the three GitHub Environments, and push to `main`.