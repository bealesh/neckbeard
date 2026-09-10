# Tranche two checkpoint — September 9, 2026

The work recorded below was subsequently committed as `aa127b7` on September 10.
Descriptions of uncommitted work are historical. See the [launch plan](../launch-plan.md)
for the remaining acceptance gates; this checkpoint is not a completed release.

Resumed after the user's pause. Work is uncommitted on `feat/tranche-2-deployment`,
based on merged main `61ba2916ff31aa23aaa0859523e94f7f4cf35411`. This is an
implementation checkpoint, **not a completed tranche or release**.

## Acceptance target

Exercise first deployment, update, digest-preserving promotion, and rollback
across AWS/GCP/Azure × GitHub/GitLab × serverless/Kubernetes (12 combinations).
Each live test must prove HTTPS health, the running release identity, and real
database connectivity. Manually repaired deployments do not count.

## Verified at this checkpoint

- Full Go tests and vet pass; whitespace checks pass after a catalog formatting fix.
- All 36 environment/bootstrap roots across six cloud/runtime configurations
  pass provider validation. All six generated standalone release runners build
  for Linux/amd64. GitHub workflow syntax checks pass for the AWS/GCP fixtures.
- Checkov passes the 12 dev environment/bootstrap roots. Kubernetes validation
  passes all 141 rendered resources, with no missing schemas or skipped objects.
- Foundation creation, durable cloud receipts, registry authentication, Flux
  approval-bound commit pins, GCP/Azure database retry handling, and infrastructure
  input wiring have local regression coverage. See `docs/tranche-2.md` for the
  implementation and migration notes.
- **Full lifecycle acceptance: 0 of 12 passed.** AWS/GitHub/serverless now has
  a live deployment with HTTPS/database health and a proven worker effect.
  Update and rollback passed; promotion and the remaining lanes are incomplete.

## Existing test resources and pending user steps

The authoritative non-secret resource inventory is
`/tmp/neckbeard-tranche2/resources.json`. The paused backup predates resumed work;
do not restore it over the live fixtures or their bootstrap state.

- AWS account `656253649226`, region `us-east-2`; temporary CLI login works.
  Route53 zone `neckbeard.beale.sh` (`Z063685638MFE659WVD3V`) exists, with a
  validated wildcard ACM certificate. The dedicated subdomain is now delegated through Netlify; all four NS records were verified at its authoritative nameserver (TTL 300).
  AWS foundations and a private database now exist; the hosted zone, private
  endpoints, database, load balancer and app workloads are billable until teardown.
- DNS: Netlify sign-in is complete. Only the four `neckbeard` NS records were
  added; the original six website/mail records were preserved and verified.
- GCP project `neckbeard-release-test-260909`, billing attached and required APIs
  enabled; bootstrap, foundations and a private database are prepared. App creation
  partially applied; Cloud Run ingress/job IAM is blocked pending explicit approval
  for one project-scoped Cloud Run Admin grant. Do not use unrelated Stock projects.
- Azure subscription `151e4cda-9c99-4e21-8f97-25b36eaf785d`; empty resource group
  `neckbeard-release-tests` exists in eastus2. The explicitly approved bootstrap
  in centralus completed after a generated recovery (see below). Its nine-resource
  foundation and private PostgreSQL setup also completed. Initial app HTTPS,
  database and worker probes passed; repeat-run recovery is underway (see below).
- GitHub private test repository `bealesh/neckbeard-release-tests` exists, with
  custom OIDC subject claims and main-only dev branch policy. The exact immutable
  repository subject was verified in run `34411911781`; AWS trust was corrected.
  App tests and policy checks passed. The first image scan caught CVE-2026-56852;
  `golang.org/x/text` is now 0.39.0 (required x/sync 0.21.0). Patched build
  `34412617777` passed. Generated infrastructure run `34412937032` deployed
  the pinned image; HTTPS and database/worker checks passed. Generated release
  run `34413406821` passed native deployment, digest/HTTPS verification and
  durable receipt save. The direct cron task probe also passed. Update run
  `34414097071` passed for bellwether 0.2.0, retaining the original database
  record. Rollback run `34414731342` passed, restoring 0.1.0 with the original record
  intact and a fresh record processed by the worker. Production gates remain incomplete.
- GitLab project `86276082` was successfully transferred, with explicit user
  approval, to `stock-group7961837/neckbeard-release-tests`. Its custom OIDC
  subject configuration survived the transfer. Dev/stg/prd environments exist;
  prd is protected with one Maintainer approval required.
- GitLab account verification is complete. New pipeline `2834915020` at
  preflight commit `2d6461f5e5c70a25620e8d24a8a94dc3f4529247` passed dev/stg
  identity checks. Prd deployment `1561445346` is blocked with one pending
  Maintainer approval. Playing the job before approval was refused; self-approval
  was also refused. The follow-up test passed using a temporary bot initiator and the signed-in
  Maintainer approver: pipeline `2835145821`, deployment `1561514198`, job
  `16407719170`. Unapproved execution and bot self-approval were denied;
  separate-account approval was accepted and the job passed. Project token
  `27416451` was revoked and confirmed inactive. This tests separate platform
  identities, not independent human review.
- GitHub rejected the private repository's required-reviewer rule with HTTP 422
  due to its billing plan. An Enterprise-enabled private repository or an
  approved public test fixture is needed. The new `configure-ci -env prd` guard
  refused this real repository before publishing production variables.

## AWS bootstrap: explicitly approved and applied

Generated test fixture:
`/tmp/neckbeard-tranche2/aws-bootstrap-review-5v_m7uvn`.
App prefix `nbt260909-awsghsc-dev`; repository is the private GitHub test repo.
The dev bootstrap plan proposed 13 additions, zero changes, zero deletions.
No existing account OIDC providers were found when planning.

The user explicitly approved the account-wide PowerUserAccess test grant after
reviewing the refreshed plan. Apply succeeded: **13 added, 0 changed, 0 destroyed**.
The scratch resource inventory records the created bucket, CI role ARNs and OIDC
provider through the bootstrap outputs. `infra/envs/dev/backend.hcl` was written
from those outputs. Keep the bootstrap state until teardown is complete.

AWS CLI login-session credentials are bridged to the provider in memory; never
print them or place them in command arguments. The application stack is live.
The test DNS delegation and wildcard ACM certificate are active. HTTPS endpoint:
`https://awsghsc-dev.neckbeard.beale.sh/healthz`.

GCP bootstrap completed with 14 additions, followed by seven foundation resources
and the prepared private PostgreSQL connection. Fixture:
`/tmp/neckbeard-tranche2/gcp-bootstrap-review-m43i2zuh`.

AWS foundations added 25 resources. Database setup added five resources and
saved its private connection; retries reused the cloud-stored password. Live
checks fixed AWS CLI's double-read of JSON stdin and the provider's conflict
between `password_wo` and an explicit false managed-password flag. Fresh generated
AWS databases use a stable application password; RDS-managed rotation remains
the default for direct module consumers. AWS bootstrap trust was updated in place
for GitHub's verified immutable subject (two role updates, unchanged permissions).

Azure fixture: `/tmp/neckbeard-tranche2/azure-bootstrap-review-4kfujdka`.
The user explicitly approved its 16-resource bootstrap including Contributor and
RBAC Administrator on the test subscription. An initial partial apply hit storage
key authentication and flexible-credential errors. Generated providers now use
Entra storage auth, container creation waits for operator data access, and trust
uses GitHub's exact immutable subject. Recovery succeeded: six additions, two
updates, replacement of the empty tainted storage account. Local state and backend
outputs are retained. Foundation and database setup succeeded; connection secrets
and passwords were verified absent from infrastructure state on all three clouds.
For local CLI-based
backend access, pass ARM_SUBSCRIPTION_ID without ARM_TENANT_ID: passing both
causes the Azure CLI token request to fail. Federated CI uses its OIDC path.

## Remaining implementation work

1. Prove the remaining live app/database connections and finish secret population.
2. Exercise the generated first-deployment sequence, including first-image CI,
   environment HTTPS inputs and database grants. All three database setups passed;
   the first AWS application is live. Finish native releases, update and rollback.
3. Finish Kubernetes bootstrap/private-repository credentials, CI cluster access,
   HTTPS. Environment ConfigMaps now supply APP_ENV to all workloads. Prove worker
   effects and execute cron jobs.
4. Complete repository gates and verify cloud-enforced read/write separation,
   including denied refs/environments and a successful independent prd approval.
5. Build and execute the full 12-combination live harness, record actual evidence
   and clean up owned resources. Manually repaired deployments do not count.

The scratch inventory is `/tmp/neckbeard-tranche2/resources.json`. A restricted resumed backup is at
`/Users/dbeale/workspace/oss/neckbeard-checkpoints/tranche-2-2026-09-09-resumed`;
it includes all three fixtures and their bootstrap state. Remote application
state remains authoritative. The original paused backup predates resumed changes. No automated follow-up
or background deployment has been scheduled.

## Follow-up live findings

GCP and Azure now have separate private GitHub fixtures,
`bealesh/neckbeard-release-tests-gcp` and
`bealesh/neckbeard-release-tests-azure`, with updated bootstrap trust. Both first
image builds passed tests, scanning and publishing. GCP's CI apply identity has
Secret Manager Admin on the single database connection secret, allowing runtime
access bindings; its redundant direct accessor grant was removed. The next apply
failed on Cloud Run service/job IAM. One `roles/run.admin` project grant is planned
but **not applied**: automatic approval review rejected it pending explicit user
approval. The public GitHub gate-only fixture also remains unpublished pending
permission.

Azure first app infrastructure run `34416205260` succeeded. Its HTTPS response
verified version 0.1.0, environment dev and database access; a created note was
processed by the worker. The next run `34416654463` planned four replacements
because Azure's generated `infrastructure_resource_group_name` read back as an
optional replacement-triggering value. All three application workloads were deleted before cancellation; Azure had
also scheduled environment deletion, discovered during recovery. The database
survived. The run and queued release `34416908230` were canceled.
The catalog now retains Azure's generated name. A refreshed recovery plan proposes
three workload creations, zero changes and zero deletions; generated recovery run
`34417965294` is underway. No Azure cron execution has passed yet.

Provider failures now report redacted OpenTofu diagnostics instead of only an exit
code. Full Go tests and vet passed after that change; tests required local socket
access. Full lifecycle acceptance remains **0/12**.

Recovery follow-up: cancellation left an Azure state lock, which was cleared only
after confirming the owning run and all other writers had stopped. The next
attempt exposed that Azure had already marked the environment `ScheduledForDelete`
before cancellation. Retrying deletion of that empty environment completed; its
API now returns ResourceNotFound. No database or virtual-network deletion was
performed. Recovery run `34417965294`, attempt 3, is recreating the environment and
workloads. The platform HTTPS hostname may change and must be refreshed before
release verification. The Azure database secret and password were again verified
absent from application state.

Azure recovery attempt 3 completed successfully. The new HTTPS host is
`nbt260909-azghsc-dev-web.orangebeach-983a6a0b.centralus.azurecontainerapps.io`.
The original note survived (its evidence keeps both endpoint names), and the
scheduled job executed the pinned image and read that populated database. The
repeat plan returned zero changes. Native release `34418916994` also passed and
saved its verified receipt. Azure update image build `34419064819` is underway.

AWS deliberate failure run `34418049045` failed on the expected version mismatch,
with the cloud receipt byte-for-byte unchanged. The original note was still present
under 0.2.0. The CLI then selected the most recent verified 0.1.0 release for recovery;
run `34419018017` is deploying that rollback.

AWS failure recovery `34419018017` passed, including original-record persistence
and fresh worker processing (note 3). Azure update `34419295836` passed at 0.2.0,
retaining note 1 and processing note 2. Post-update provider planning again found
zero changes. GitHub canceled queued Azure rollback `34419576390` when another
infra job arrived; generated infra/release concurrency now uses `queue: max`.
The latest actionlint release lacks that supported field, so only its exact queue
key diagnostic is excluded, with the upstream issue documented in tranche notes.
The replacement Azure rollback is `34419784813` at `fab7caa`, currently running.

Final resumed checkpoint: Azure rollback `34419784813` succeeded at 0.1.0,
retaining both existing records and processing fresh note 3. AWS and Azure latest
infra, app-build and release workflows are all complete and successful. The only
canceled Azure release is the superseded pre-queue-fix dispatch. Full Go tests,
vet and formatting pass. Six cloud/runtime provider fixtures and Linux runners
passed before the queue-only workflow change; renderer tests and live GitHub
execution verify the latest workflow syntax, with the documented actionlint
queue-field exception. No cloud deployments are left running.

Tranche two is still incomplete: full lifecycle acceptance is 0/12 because
promotion, Kubernetes and remaining cloud/VCS lanes have not passed. The full
automatic live harness is unfinished. GCP's project-scoped Cloud Run Admin grant
is still unapplied pending explicit user approval after automatic approval review
rejected it. The public GitHub gate-only fixture remains unpublished pending
permission. No tranche-two plugin commit or PR has been created. Cloud resources
remain live and their bootstrap state is retained for continuation and teardown.
