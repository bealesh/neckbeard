Done. Nothing was deployed and no cloud API was called; the only network call was Infracost's pricing API for list prices. All files are uncommitted in the working tree for your review.

**What I generated**

- `neckbeard.yaml` and `app-profile.yaml`: the two reviewed inputs. The profile records every fact with file:line evidence and every assumption with its resolution.
- `infra/envs/{dev,stg,prd}/`: one OpenTofu root per environment composing eight pinned catalog modules. Per environment: VPC across two AZs with private subnets and no NAT gateway, public ALB with health checks, ECS Fargate service at 0.5 vCPU / 1 GB with CPU autoscaling, RDS PostgreSQL 16 on db.t4g.micro (encrypted, private, prd has deletion protection), a Secrets Manager container for `DATABASE_URL` injected as an ECS secret, ECR with immutable tags and scanning, CloudWatch logs, and a tag-scoped AWS Budget.
- `infra/bootstrap/{dev,stg,prd}/`: remote state (S3 with versioning, KMS, lockfile) plus a GitHub OIDC provider and two IAM roles per environment, read-only for PR plans and write for applies on `main`.
- `infra/envs/*/custom.tf` and `https.auto.tfvars`: the HTTPS layer. ACM certificate, Route 53 validation, CNAME to the ALB, and the TLS 1.3 listener with HTTP redirect. It needs a hostname you own, so it stays dormant until you fill in the tfvars.
- `.github/workflows/`: `neckbeard-ci.yml` tests and builds the image with Trivy scanning and pushes by digest. `neckbeard-infra.yml` runs checkov, then read-only plans on PRs and applies on `main`, gated by GitHub environments. `neckbeard-release.yml` deploys a committed digest and keeps rollback receipts. All cloud access is OIDC federation; no keys anywhere.
- `costs/estimate.md`: the full cost report with every usage assumption.
- `INFRASTRUCTURE.md`: the reviewer's entry point.

**Where to start**: `INFRASTRUCTURE.md`, then `neckbeard.yaml`, `app-profile.yaml`, `docs/topology.md`, `infra/envs/prd/main.tf`, then the modules under `.neckbeard/catalog/`.

**Validation**: `tofu fmt`, `init`, `validate`, checkov and actionlint all pass across the six roots. The authenticated plan and deployment are not exercised; passing static checks proves syntax and policy conformance, not deployability.

**Monthly cost estimate** (us-east-1 list prices, no free tier or discounts)

| environment | idle baseline | expected | high (2x traffic) |
|---|---|---|---|
| dev | 130.53 | 136.51 | 160.51 |
| stg | 130.53 | 136.51 | 160.51 |
| prd | 130.53 | 136.51 | 160.51 |
| total USD | 391.58 | 409.52 | 481.53 |

All three environments cost the same because the solo tier sizes them identically. The biggest items per environment are four VPC interface endpoints (58.80), the ALB (23.90), Fargate (18.02), Container Insights (15.00), and RDS (14.93). The real lever is environment count: destroying stg when not in use saves about 137 USD a month.

**Assumptions I made**

1. Solo tier: under 5 rps, single AZ, RPO 24h / RTO 8h, no on-call.
2. Fargate over Kubernetes, `us-east-1`.
3. PostgreSQL is provisioned. The app reads `DATABASE_URL` but opens no connection, so engine confidence is low. Switch to `mode: reference` if a database exists.
4. One HTTP service, no workers or cron jobs.
5. Budget alert at 450 USD total, pre-scaffold ceiling at 500.
6. Placeholders marked `REPLACE`: org `myorg`, repo `YOUR-GH-OWNER/YOUR-REPO`, budget email. The repo slug is baked into OIDC trust, so fix these, re-run `neckbeard plan && neckbeard scaffold`, then commit.

**Next steps**: `docs/bootstrap.md` walks through the one-time human steps per environment with elevated credentials: apply the bootstrap root, commit `backend.hcl`, create foundations and the database, publish CI variables, and activate the cost-allocation tags so the budgets match spend.