Done. Everything is in the working tree, uncommitted, and nothing touched a cloud API.

**What I generated**

- **Application profile and config.** `app-profile.yaml` records the three process roles from the one image (`web` HTTP on 8080 with `/healthz`, `worker` loop, `report` daily cron) with file:line evidence, one PostgreSQL need, and one secret name. `neckbeard.yaml` picks AWS us-east-1, GitHub, ECS Fargate, tier `solo`, a USD 500 ceiling and USD 160 per-environment budget alerts. `blueprint.yaml` is the hash-locked plan.
- **OpenTofu.** Three environment roots under `infra/envs/` wiring seven pinned catalog modules: VPC with private subnets and interface endpoints instead of NAT, internet-facing ALB with HTTPS and HTTP-to-HTTPS redirect once a certificate ARN is supplied, ECS Fargate services plus an EventBridge cron, RDS PostgreSQL 16 with encrypted storage and deletion protection in prd, Secrets Manager, ECR with immutable tags, and AWS Budgets. Three bootstrap roots under `infra/bootstrap/` create the versioned, encrypted S3 state bucket, the GitHub OIDC provider, and read-only plan and prefix-scoped apply roles per environment. Each environment's user-owned `custom.tf` adds an optional ACM certificate request gated on a hostname.
- **Pipelines.** `neckbeard-ci.yml` runs the test hook, builds the image, Trivy-scans it, and pushes an immutable digest. `neckbeard-infra.yml` runs Checkov and a read-only plan on pull requests, then applies on `main`. `neckbeard-release.yml` deploys a reviewed digest per environment behind GitHub environment gates. All AWS access is OIDC federation.
- **Docs.** `infra/README.md` is the review guide. `docs/cost-estimate.md` is the cost narrative. `docs/bootstrap.md` is the generated operator runbook.

**Verification.** Static checks passed on all six roots: formatting, provider init and validate, Checkov, and actionlint. The test hook runs clean locally. An authenticated plan and any apply were not exercised, since there are no credentials.

**Where a reviewer should start.** `README.md` at the root, then `infra/README.md`, which gives the reading order, the per-environment table, the identity model, and the assumptions table. Challenge `app-profile.yaml`'s `confirmed` answers first, since I wrote them on your behalf.

**Estimated monthly cost.** List prices for us-east-1 via Infracost, no free tier or tax.

| | expected USD/month |
|---|---|
| dev | 154.54 |
| stg | 154.54 |
| prd | 154.54 |
| all three | 463.62 |

About 96 percent of that is fixed. The four VPC interface endpoints are the largest line, then the ALB, the always-on worker and web tasks, Container Insights, and RDS. Switching to a single NAT gateway drops the total to about 387. Running prd alone and tearing down dev and stg between uses costs about 155. The smallteam tier starts above 673 with a partial estimate.

**Assumptions you must confirm before bootstrap.** The GitHub repository slug is `stockitbetter/bellwether`, and OIDC trust binds to it. The org prefix is `sib`. The region is us-east-1. The database is provisioned new. The report job runs daily at 06:00 UTC. The budget alert address is a placeholder. Public hostnames are not set, so the ALB serves HTTP only until you follow the three steps at the top of each `custom.tf`.