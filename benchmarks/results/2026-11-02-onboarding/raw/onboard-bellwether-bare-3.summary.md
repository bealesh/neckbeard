Everything is generated and verified offline. Nothing was deployed and no cloud API was called.

**What I generated**

- `infra/` holds OpenTofu for three environments plus a one-time bootstrap stack. Each environment gets its own VPC, an HTTPS Application Load Balancer with an ACM certificate and Route53 record, ECS Fargate services for the `serve` and `work` modes, an EventBridge schedule that runs `report` daily, RDS PostgreSQL 16 in private subnets with TLS forced, the `DATABASE_URL` secret in Secrets Manager injected at task start, CloudWatch logs, and five alarms to an SNS email topic.
- `infra/bootstrap/` creates the versioned, encrypted S3 state bucket (S3-native locking, no DynamoDB), the GitHub OIDC provider with one read-only plan role, one ECR push role, and one deploy role per environment, the ECR repository with immutable tags, and a monthly AWS Budget.
- `.github/workflows/` has three workflows. `ci.yml` runs Go tests and a no-push image build on pull requests. `infra-plan.yml` formats, validates, and plans all three environments on pull requests and posts a sticky comment. `deploy.yml` builds and pushes the ARM64 image tagged with the git SHA on `main`, then applies dev, stg, and prd in sequence through GitHub Environments. No AWS keys are stored anywhere.
- `infra/COST.md` has the itemized estimate and levers. `infra/README.md` is the reviewer guide.

**Fix outside the ask**: the committed Dockerfile was corrupted (mangled `go build -o`, `COPY`, and `ENTRYPOINT` lines). I repaired it, made it cross-compile for ARM64, and built and smoke-tested the image locally in both `serve` and `report` modes.

**Verification**: all four stacks pass `tofu fmt -check` and `tofu validate` with AWS provider 6.71, the lock files carry Linux and macOS hashes, and the workflows pass actionlint. Nothing beyond that could be verified without an AWS account.

**Where a reviewer should start**: `infra/README.md`, then diff `infra/envs/prd/main.tf` against `infra/envs/dev/main.tf` to see every intentional difference, then `infra/modules/environment/main.tf` for the wiring, and `infra/bootstrap/github_oidc.tf` for who can assume what.

**Estimated monthly cost** (us-east-1, on-demand prices, ±15%):

| Environment | $/month |
|---|---:|
| dev (Spot tasks, db.t4g.micro, no NAT) | 51 |
| stg (same as dev) | 51 |
| prd (2 on-demand serve tasks, db.t4g.small, NAT gateway) | 131 |
| shared (Route53, ECR, state) | 1 |
| **Total** | **235** |

The fixed costs dominate: three load balancers with their IPv4 addresses, the prd NAT gateway, and three databases. Destroying dev when idle saves $51, and dropping the prd NAT gateway saves $23.

**Assumptions you should confirm or change**

- One AWS account and region (`us-east-1`) for all environments. Per-environment accounts are the recommended upgrade, since in one account the per-environment deploy roles are a convention, not a boundary.
- A Route53 public hosted zone already exists. Hostnames are `example.com` placeholders in `infra/envs/*/main.tf`.
- Low traffic, single-AZ databases everywhere, no WAF or CloudFront. Multi-AZ for prd is a one-line change costing about $26 more.
- dev and stg tasks run in public subnets with a public IP but admit traffic only from the load balancer; prd tasks are private behind NAT.
- The generated database password exists in the remote state, which is private and encrypted. Moving it out of state with write-only arguments on OpenTofu 1.11+ is the first hardening follow-up listed in the README.

**Before first use**: set the hostnames, run the bootstrap apply with your own admin credentials, put the bucket name in `infra/backend.hcl`, add the `AWS_ACCOUNT_ID` and `AWS_REGION` repository variables, and create the three GitHub Environments with a required reviewer on prd. The changes are uncommitted in the working tree for your review.