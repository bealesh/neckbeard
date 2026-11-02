Everything is generated, validated and left uncommitted in the working tree. Nothing was deployed and no AWS API was called.

## What I generated

**Infrastructure (`infra/`, OpenTofu, AWS provider 6.x).** All four roots pass `tofu fmt -check` and `tofu validate` against the real provider schemas, with provider lock files covering Linux and macOS.

- `bootstrap/` is applied once per account. It creates the versioned, encrypted S3 state bucket using the native S3 lock file instead of DynamoDB, the shared ECR repository with immutable tags, the Route 53 hosted zone, and the GitHub OIDC provider with three roles: `plan` is read-only, `publish` can only push images, and `apply` is PowerUser plus IAM scoped to the `/bellwether/` role path.
- `modules/` holds five small modules: network, ingress, database, app and monitoring.
- `stack/main.tf` composes them into one environment, with the ALB, task and database security-group boundary written out in one place.
- `envs/dev|stg|prd/` are thin roots. Every per-environment decision is a literal argument, so diffing `prd/main.tf` against `dev/main.tf` shows exactly what production adds.

**Pipeline (`.github/workflows/`).** OIDC only, no stored cloud keys. Both workflows pass `actionlint`.
- `ci.yml` runs on pull requests: Go vet/build/test, an arm64 image build, fmt and validate, then a plan for each environment posted as a sticky PR comment. Forked PRs skip the plan since they get no OIDC token.
- `deploy.yml` runs on pushes to `main`: builds and pushes the image once tagged with the git SHA, then applies dev, stg and prd in sequence through the reusable `_deploy-env.yml`. ECS waits for steady state with a rollback circuit breaker, so a bad image fails the apply and stops promotion. A manual dispatch redeploys any environment at any earlier SHA, which is the rollback path.

**Cost estimate** is in `infra/COSTS.md`, with unit prices and sizing assumptions written down.

**One repo fix.** The committed Dockerfile could not build (truncated output path, `COPY --from=build..`, invalid `ENTRYPOINT`). I repaired it, added cross-compilation so arm64 builds fast on x86 runners, and verified the image builds, runs as nonroot, answers `/healthz`, accepts a note and runs `report`.

## Key assumptions

- One AWS account for all three environments, region `us-east-1`, three VPCs with distinct CIDRs.
- ECS on Fargate ARM64 for the three modes: `serve` behind an ALB, `work` as a service, `report` via EventBridge Scheduler daily.
- RDS PostgreSQL 17 with TLS enforced. The password is generated in OpenTofu and delivered as a complete `DATABASE_URL` in Secrets Manager, because the app reads exactly that variable. The trade-off that it also lives in encrypted state is documented, as is the rotation command.
- dev and stg skip the NAT gateway and run tasks in public subnets admitting only the ALB. prd uses one NAT gateway and private subnets. One variable flips either.
- The hosted zone is created by bootstrap; registrar delegation and the `root_domain` placeholder in `envs/*/terraform.tfvars` are yours to fill in.

## Monthly cost

| Environment | USD/month |
|---|---:|
| dev | 64 |
| stg | 64 |
| prd | 133 |
| shared | 1 |
| **Total as configured** | **≈ 262** |

Roughly 60% is fixed plumbing (three ALBs, public IPv4 addresses, the prd NAT gateway). Dropping the prd NAT gateway and scaling dev to zero when idle brings it to about 215. Destroying dev when unused brings it near 175. Prices are list prices from memory as of today and should be checked against the AWS Pricing Calculator.

## Where a reviewer should start

1. `infra/README.md` for the architecture diagram, decision table and bootstrap steps.
2. `infra/stack/main.tf`, then `infra/envs/prd/main.tf` beside `infra/envs/dev/main.tf`.
3. `infra/bootstrap/github_oidc.tf` for CI permissions.
4. `.github/workflows/deploy.yml` and `_deploy-env.yml` for the promotion flow.
5. `infra/COSTS.md`.

Before the first deploy you need to run bootstrap with admin credentials, delegate the domain, set the `AWS_ACCOUNT_ID` and `AWS_REGION` repository variables, and create the `dev`, `stg` and `prd` GitHub Environments with a required reviewer on `prd`. Follow-ups I noted but did not do: pin actions to commit SHAs, add tflint or checkov, and consider WAF and access logs before real traffic.