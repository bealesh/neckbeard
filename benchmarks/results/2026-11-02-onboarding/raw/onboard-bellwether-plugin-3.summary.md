Done. Everything is generated and statically validated in the working tree; nothing was committed, deployed, or sent to any cloud API.

**What was generated**

- `neckbeard.yaml` and `app-profile.yaml` hold the reviewed decisions. The profile records the three process modes found in `main.go` (HTTP `serve`, always-on `work`, daily `report`) and Postgres as the only datastore, with file and line evidence.
- `infra/bootstrap/{dev,stg,prd}` creates the remote state bucket (S3, versioned, KMS), the GitHub OIDC provider, and a read-only plan role plus an apply role per environment.
- `infra/envs/{dev,stg,prd}` is the runtime stack per environment: private VPC across two AZs with one NAT gateway, public ALB with `/healthz` checks, ECS Fargate for the three services, RDS PostgreSQL 16 in private subnets, a Secrets Manager container for `DATABASE_URL`, ECR with immutable tags, and an AWS Budget. Prd adds deletion protection and a final snapshot.
- `.github/workflows/` has three workflows: CI runs the test hook on PRs and builds, Trivy-scans and pushes by digest on main; infra runs checkov then a per-environment OpenTofu plan on PRs and apply on main; release is a manual, gated deploy of a committed digest. All cloud access is OIDC federation with no stored keys.
- `.neckbeard/hooks/test.sh` now runs `go vet`, `go build` and `go test`, and passes locally.
- `costs/estimate.md` is the cost report. `docs/infra-review.md` is the reviewer guide with the full assumption table.

Validation passed all 24 V0 checks: fmt, init and validate for six roots, checkov for six roots, actionlint for three workflows. Authenticated plans and deployment were not exercised, by design.

**Where a reviewer should start**

1. `neckbeard.yaml`, then the `confirmed` block of `app-profile.yaml`.
2. `docs/infra-review.md` for the reading order, posture, and assumptions.
3. `infra/envs/prd/main.tf` and the vendored modules under `.neckbeard/catalog/`.
4. `docs/bootstrap.md` for the first human step.

**Estimated monthly cost, USD, list prices, us-east-1**

| | idle baseline | expected | high (2× usage) |
|---|---|---|---|
| per environment | 123.00 | 129.04 | 153.11 |
| all three | 368.99 | 387.12 | 459.32 |

The main drivers per environment are the NAT gateway, the ALB, two always-on Fargate tasks, Container Insights, and RDS. The solo preset's no-NAT shape was more expensive here, so I applied the one override the catalog allows for that input. Dropping stg by simply not bootstrapping it would bring the footprint to roughly 258 per month.

**Assumptions you must correct before bootstrap**

- Repo slug `stockitbetter/bellwether` and org prefix `sib`: there was no git remote, and OIDC trust binds to the exact slug.
- Budget alert address is a placeholder `budget-alerts@example.com`.
- Tier `solo`, region `us-east-1`, a newly provisioned RDS per environment, and a daily 06:00 UTC report schedule.

**One gap to know about**

HTTPS on the ALB renders only once an ACM certificate ARN is set per environment through `infra/envs/<env>/https.auto.tfvars`. The catalog does not create Route 53 zones or certificates, so until then the listener is plain HTTP on port 80. The exact steps are in the reviewer guide.