Done. Everything is generated, statically validated, and uncommitted in the working tree for your review. Nothing was deployed and no cloud API was called.

## What was generated

The app is one HTTP service on port 8000 with a `/healthz` probe that reads `DATABASE_URL`. I used the neckbeard plugin for the deterministic half (catalog modules, plan, estimate, scaffold, validate) and hand-wrote only the HTTPS extension, the test hook, and the reviewer guide.

- **OpenTofu, three environments.** `infra/envs/{dev,stg,prd}/` each compose seven catalog modules: VPC with private subnets and one NAT gateway, internet-facing ALB, ECS Fargate (one 0.5 vCPU task, autoscaling, circuit-breaker rollback), RDS PostgreSQL 16 on db.t4g.micro with the password kept out of state, Secrets Manager container for `DATABASE_URL`, ECR with immutable tags and scan-on-push, and an AWS Budget per environment. prd adds database deletion protection and a final snapshot.
- **Remote state and CI identity.** `infra/bootstrap/<env>/` creates a versioned, KMS-encrypted S3 state bucket, the GitHub OIDC provider, a read-only plan role and a prefix-scoped apply role.
- **HTTPS.** The catalog's ingress already switches to a TLS 1.3 listener with a 301 redirect when a certificate ARN is set. `infra/envs/<env>/custom.tf` adds the missing pieces: a DNS-validated ACM certificate and a CNAME in an existing Route 53 zone, inert until `https.auto.tfvars` is filled. It takes two applies, and the file explains why.
- **GitHub Actions.** The ci workflow runs the test hook, then on `main` builds, Trivy-scans, and pushes the image by digest. The infra workflow runs checkov, then read-only plans on PRs and applies on `main` through per-environment GitHub environments. The release workflow deploys a committed digest and keeps a rollback receipt. Credentials are OIDC only, no stored keys.
- **Cost report.** `costs/estimate.md` with line items, scenario ranges, and every usage assumption.

## Where a reviewer should start

`README.md` has a numbered reading order. The short version is `app-profile.yaml` for the evidence and assumptions, `neckbeard.yaml` for the choices, `infra/envs/prd/main.tf` for the composed stack, and `costs/estimate.md` for the money.

## Estimated monthly cost

| environment | idle baseline | expected | high (2× usage) |
|---|---|---|---|
| dev | $104.98 | $111.01 | $135.06 |
| stg | $104.98 | $111.01 | $135.06 |
| prd | $104.98 | $111.01 | $135.06 |
| total | $314.93 | $333.02 | $405.18 |

Per environment the NAT gateway, ALB, Fargate task, Container Insights, and RDS make up about $105 of that. The catalog's default for the solo tier replaced NAT with four interface endpoints, which cost more and left the app with no outbound internet, so I overrode it. ECS never scales to zero in this catalog, so the always-on task hours are real. Prices are us-east-1 list prices from Infracost on 2026-11-02, excluding tax, free tier, and GitHub Actions minutes.

## Assumptions you should check

- **The GitHub slug** `stockitbetter/app` is a placeholder because the repo has no remote. OIDC trust binds to it exactly, so fix it in `neckbeard.yaml` and re-run plan, estimate, scaffold, and validate before bootstrap.
- Tier `solo` means single-AZ production with RPO 24h and RTO 8h. Switching to `smallteam` roughly adds $100 per environment for multi-AZ.
- A new Postgres instance is provisioned per environment. The code only reads the variable, so if a database already exists, change the need to `reference`.
- No workers or cron jobs exist. Region is us-east-1. Budget alerts go to your email. Each of these has a one-line change location in the README.

## What is not proven

V0 checks passed on all six roots: format, init, validate, checkov, and actionlint. The smoke-test hook ran locally and answered on `/healthz`. An authenticated plan and an actual deployment were not exercised and need the bootstrap steps in `docs/bootstrap.md`, which a human runs with elevated credentials.