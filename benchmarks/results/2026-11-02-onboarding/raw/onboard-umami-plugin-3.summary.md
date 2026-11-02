Done. The repository now holds reviewable, statically validated infrastructure for Umami on AWS. Nothing was deployed and no cloud API was called.

## What was generated

The app is Umami 3.4.1, a single Next.js process on port 3000 with a health endpoint at `/api/heartbeat`, backed by PostgreSQL. Redis, ClickHouse, and Kafka exist in the code but each is gated on its own environment variable, so they are recorded as not required with the gating lines cited.

- **Inputs you own:** `neckbeard.yaml` (cloud, tier, one cost override, budget) and `app-profile.yaml` (every claim about the app with file and line evidence).
- **OpenTofu:** `infra/envs/{dev,stg,prd}` roots, each with VPC, ECS Fargate service, ALB, RDS PostgreSQL 16, Secrets Manager containers, ECR, CloudWatch, and an AWS Budget. `infra/bootstrap/{dev,stg,prd}` creates the S3 remote state bucket, the GitHub OIDC provider, and read-only plan plus scoped apply roles. Module sources are vendored under `.neckbeard/catalog/`.
- **GitHub Actions:** `neckbeard-ci.yml` tests on every PR, then builds, Trivy-scans, and pushes the image on `main`. `neckbeard-infra.yml` runs checkov and per-environment plans on PRs and applies on `main`. `neckbeard-release.yml` deploys a committed digest by hand-picked environment. All cloud access is OIDC via GitHub environment variables. No access keys anywhere.
- **Cost:** `costs/estimate.md` from Infracost, plus the reviewer guide below.
- **Verification:** `tofu fmt`, `init`, `validate`, checkov, and actionlint all passed on all six roots. Authenticated plan, apply, image build, and TLS are not exercised.

## Where a reviewer should start

Open `docs/infra-review.md`. It gives the reading order, the resource table, the secrets and HTTPS wiring, the pipeline trust boundary, every assumption with how to change it, and the cost breakdown. From there, `neckbeard.yaml`, then `infra/envs/prd/main.tf`.

## Estimated monthly cost

| | per environment | three environments |
|---|---|---|
| Baseline, idle | 105.78 | 317.33 |
| Expected | 111.82 | **335.45 USD** |
| High, 2× usage | 135.88 | 407.64 |

Biggest lines per environment are the NAT gateway, the load balancer, the always-on Fargate task, and Container Insights. List prices, no free tier, no discounts. The NAT gateway's public IPv4 charge is unmodeled, about 3.65 USD per environment.

## Assumptions that need your attention

- **Three placeholders** in `neckbeard.yaml` must be edited before bootstrap: the org slug, the repo slug (baked into the OIDC trust), and the budget alert email. Then re-run plan, estimate, scaffold, validate.
- **Solo tier** was assumed: under 5 rps, single-AZ, 24-hour RPO. The smallteam tier roughly doubles cost.
- **One NAT gateway** replaces the solo preset's four VPC endpoints. This saves about 25 USD per environment but gives tasks internet egress. Delete the override for a no-egress network.
- **HTTPS** renders only once you supply an ACM certificate ARN per environment. The guide shows the exact tfvars file. Until then the ALB serves plain HTTP.
- **Dev and stg** can be torn down when idle. Running prd alone costs about 112 USD per month.

Next step is committing the untracked files and following `docs/bootstrap.md` for dev with elevated credentials.