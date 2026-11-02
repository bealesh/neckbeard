Done. Everything is generated and statically validated in the working tree, uncommitted. Nothing was deployed and no cloud API was called.

## What was generated

- **Inputs you own**: `app-profile.yaml` (what the app is, every claim cited to file:line) and `neckbeard.yaml` (AWS us-east-1, ECS Fargate, tier solo, one override, budget and ceiling).
- **OpenTofu**: `infra/bootstrap/{dev,stg,prd}` creates the S3 remote-state bucket, the GitHub OIDC provider, a read-only plan role and a scoped apply role. `infra/envs/{dev,stg,prd}` wires VPC, ALB ingress with an optional HTTPS listener, Fargate service, RDS Postgres 16, Secrets Manager containers, ECR, and an AWS Budget. Module sources are pinned under `.neckbeard/catalog/` and referenced by relative path.
- **GitHub Actions**: `neckbeard-ci.yml` tests on PRs and builds, Trivy-scans and pushes an immutable image on main. `neckbeard-infra.yml` runs checkov plus a read-only plan per environment on PRs and applies on main. `neckbeard-release.yml` is a manual, per-environment deploy of a committed digest with rollback receipts. All federate via OIDC; no stored cloud keys.
- **Cost report**: `costs/estimate.md` with every usage assumption and unpriced item listed.
- **Reviewer guide**: `INFRASTRUCTURE.md` at the repo root.

## Where to start

Read `INFRASTRUCTURE.md`, then `app-profile.yaml`, then `infra/envs/prd/main.tf` with the catalog modules beside it. `docs/bootstrap.md` is the human runbook for the one-time, credentialed setup.

## Estimated monthly cost

| Scenario | Per environment | All three |
|---|---|---|
| Idle baseline | 105.78 | 317.33 |
| Expected | 111.82 | 335.45 |
| High (2× usage) | 135.88 | 407.64 |

USD, list prices, no free tier or tax. Fixed cost dominates: NAT gateway, ALB, one always-on Fargate task, Container Insights, and the db.t4g.micro make up most of each environment. The biggest lever is running three environments at all. Dropping or pausing stg saves about 112 USD a month.

## Assumptions to confirm

- Repository slug `stockitbetter/umami` is a guess since the checkout has no remote. OIDC trust binds to it exactly.
- Org name `stockit`, region us-east-1, and tier solo (≤5 rps, single AZ, RPO 24h / RTO 8h).
- Redis, ClickHouse and Kafka are not provisioned because the code only enables each when its env var is set.
- A single NAT gateway replaces the preset's four VPC interface endpoints, saving about 25 USD per environment. Delete the `overrides` block to revert.
- HTTPS needs an ACM certificate you request. The steps are in the guide.
- Budget alerts go to your address and notify only.

## Verified and not verified

All six roots passed tofu fmt, init, validate and the checkov gate. The workflows pass actionlint except for the `concurrency.queue` key the locally installed version predates. Authenticated plan, apply, deployment and real invoices were not exercised.