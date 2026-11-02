Done. Nothing was deployed and no cloud API was called; everything is in the working tree, uncommitted.

**What was generated** (via the neckbeard CLI from two reviewed inputs, plus one hand-written extension):

- `app-profile.yaml` and `neckbeard.yaml`: the evidence-backed profile (one HTTP service on 8000 with `/healthz`, one Postgres need, no workers) and the choices (AWS us-east-1, GitHub, ECS Fargate, tier solo, single NAT override, budget alert, cost ceiling).
- `infra/envs/{dev,stg,prd}/`: per-environment roots composing VPC, ALB ingress, ECS Fargate, RDS Postgres 16, Secrets Manager, ECR, CloudWatch and an AWS Budget. `custom.tf` in each adds the ACM certificate and Route 53 records for HTTPS, dormant until a hostname is set.
- `infra/bootstrap/{dev,stg,prd}/`: S3 remote state (versioned, KMS, locked down), GitHub OIDC provider, and plan (read-only) and apply roles bound to the repo slug and `main`.
- `.github/workflows/`: image build, Trivy scan and push on main; checkov plus read-only plans on PRs and apply on main; a manual, gated release workflow by immutable digest. All AWS access is OIDC federation, no stored keys.
- `costs/estimate.md`, `docs/topology.md`, `docs/bootstrap.md`, and `INFRASTRUCTURE.md` as the reviewer guide.

**Verification:** `neckbeard validate` passed V0 on all six roots (tofu fmt, init, validate, checkov, actionlint). Authenticated plan and deployment are NOT EXERCISED by design. The test hook ran locally and the smoke test passed.

**Where a reviewer should start:** `INFRASTRUCTURE.md`, then `app-profile.yaml`, `neckbeard.yaml`, `infra/envs/prd/main.tf`, `infra/envs/prd/custom.tf`, and the bootstrap module.

**Estimated monthly cost** (Infracost list prices, us-east-1, no free tier or discounts):

| Environment | Idle floor | Expected | High (2x traffic) |
|---|---|---|---|
| dev | 104.98 | 111.01 | 135.06 |
| stg | 104.98 | 111.01 | 135.06 |
| prd | 104.98 | 111.01 | 135.06 |
| Total USD | 314.94 | 333.02 | 405.18 |

The floor is architecture rather than traffic: NAT gateway, ALB, one always-on Fargate task, Container Insights and the smallest RDS instance. The single-NAT override already cut about 76 USD/month versus the default interface endpoints. Running only prd and destroying dev/stg when idle is about 111 USD/month.

**Assumptions you should check** (all recorded in the files and cheap to change before bootstrap): repo slug `stockitbetter/app` since there is no git remote, names `stockitbetter`/`app`, region us-east-1, tier solo, a newly provisioned Postgres per environment, no background workers, your own email as the budget alert recipient, and an unknown HTTPS hostname.

**Next step** is human-only: fix the repo slug if needed, re-run plan and scaffold, then follow `docs/bootstrap.md` per environment with elevated credentials.