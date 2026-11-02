Done. The repository now holds reviewable, statically validated infrastructure as code for Umami on AWS across dev, stg and prd, plus CI/CD and a cost estimate. Nothing was deployed and no cloud API was called. Nothing is committed; the files are untracked in the working tree.

**Where a reviewer should start**

1. `docs/infrastructure.md` is the reviewer's guide: reading order, every assumption, placeholders, HTTPS, CI trust boundary, caveats.
2. `app-profile.yaml` holds what the app needs, with file:line evidence and an answer for every assumption.
3. `neckbeard.yaml` holds the operator choices and the one cost override.
4. `infra/envs/prd/main.tf` is the production root; dev and stg differ only in ceilings, backup retention and deletion protection.
5. `costs/README.md` is the cost summary with assumptions, then `costs/estimate.md` for the full report.

**What was generated**

- **Runtime and datastores, per environment:** a two-AZ VPC with one NAT gateway, an ECS Fargate service for the single `web` process with health check on `/api/heartbeat`, an internet-facing ALB, RDS PostgreSQL 16 on db.t4g.micro with encrypted storage and automated backups, three Secrets Manager containers, an ECR repository, and an AWS Budget. Modules are vendored and content-addressed under `.neckbeard/catalog/`.
- **HTTPS ingress:** the catalog ships the ALB as HTTP by default, so each environment's user-owned `custom.tf` adds an ACM certificate with DNS validation and a Route 53 alias, driven by `tls.auto.tfvars`. The first enablement is a two-step apply, documented in the file.
- **Remote state and identity:** `infra/bootstrap/<env>` creates a versioned, encrypted S3 state bucket and the GitHub OIDC provider plus read-only plan and apply roles. `docs/bootstrap.md` is the human runbook.
- **GitHub Actions:** `neckbeard-ci.yml` runs the Umami test suite on PRs and main, and on main builds, Trivy-scans and pushes the image by digest. `neckbeard-infra.yml` runs checkov and a read-only plan on PRs, and plan plus apply on main, per environment, behind GitHub environments. `neckbeard-release.yml` rolls an environment onto a committed digest. Every AWS call uses short-lived OIDC tokens; no static keys exist. Until bootstrap runs the workflows degrade to validate-only with a visible warning.
- **Validation:** `tofu fmt`, `init` and `validate` on all six roots, checkov policy, and actionlint all passed. Authenticated plans and deployment are NOT EXERCISED, by design.

**Estimated monthly cost, us-east-1 list prices**

| scenario | per environment | all three |
|---|---|---|
| Baseline, idle | 105.78 USD | 317.33 USD |
| Expected | 111.82 USD | 335.45 USD |
| High, 2× usage | 135.88 USD | 407.64 USD |

Each environment is dominated by fixed charges: NAT gateway, ALB, Container Insights, RDS and one always-running Fargate task. Switching the solo tier's four VPC interface endpoints for one NAT gateway saved about 76 USD a month across the three environments. The biggest remaining lever is not keeping dev applied, which is spelled out in `costs/README.md`. Budget alerts of 120 USD per environment notify only; the 400 USD ceiling is enforced before scaffold, not in the cloud.

**Assumptions I made for you**

- Tier `solo`: under 5 requests per second, single AZ, RPO 24 h, RTO 8 h, no on-call. One AWS account, region us-east-1, ECS Fargate rather than EKS.
- Postgres provisioned new per environment. Redis, ClickHouse and Kafka are not deployed because Umami only enables them when their URLs are set.
- The server process needs no outbound internet of its own, so one NAT is sufficient.

**Placeholders to replace before bootstrap**

- `repo: change-me/umami` in `neckbeard.yaml`. This checkout has no git remote, and the OIDC trust binds to this exact slug.
- `org: acme`, the resource-name prefix.
- `alert_emails` for budget notifications.
- The commented hostnames in each `tls.auto.tfvars`.

**One honest deviation:** infrastructure applies automatically on main, but application image rollouts do not. A push to main builds and pushes the image; moving an environment onto that digest is a reviewed commit under `releases/` plus a manual dispatch gated by the GitHub environment. That is the tool's contract and the production gate, not something I could change without hand-writing pipelines.