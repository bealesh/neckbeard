# Tranche two — deployment lifecycle (in progress)

This branch is not ready for release. The acceptance target is first deployment,
update, digest-preserving promotion, and rollback across all twelve cloud, VCS,
and runtime combinations. AWS/GitHub/serverless has passed deployment, update and rollback with data
persistence. Azure/GitHub/serverless also passed deployment, update and rollback,
including recovery after an unintended environment replacement; the fixed
configuration produces no-change repeat plans. No combination has completed the full
lifecycle acceptance yet.

## Implemented behavior

The first-deployment path now separates foundation creation from workloads:

```sh
neckbeard release foundation-plan -env dev
# Review the saved plan and resource costs.
neckbeard release foundation-apply -env dev
```

The foundation step creates the network, registry, secret containers and optional
object storage. Kubernetes foundations also create the cluster and its nodes,
which incur charges. It rejects updates, replacements, deletions and application
workloads. Managed PostgreSQL on all three clouds then uses
`prepare-database -env dev`: a random
password is saved in the existing connection secret before database creation,
so retries reuse it. A temporary `pending.invalid` connection cannot pass the
infrastructure runner. Successful setup replaces it with the private endpoint.
Existing connections pointing elsewhere are refused rather than overwritten.
Run one setup per environment at a time; concurrent setup across separate
checkouts is not supported.

Both CI systems now use `infra-plan` and `infra-apply` to supply the committed
image and ephemeral database password. PR plans use a public placeholder for
the write-only password and do not read its secret value. These database users
currently have administration privileges inside their isolated application
database instance. Credential rotation and finer database grants remain open.
Fresh generated AWS databases use this stable password instead of RDS-managed
automatic password rotation. Direct AWS module consumers retain RDS-managed
credentials by default. Migrating an existing database requires an explicit
credential change; the create-only setup helper refuses database updates.

The CLI and generated CI share the same Go release runner. Release intent lives
in `releases/<env>.json`; images must use immutable SHA-256 digests. Application
CI publishes `built-image.txt` after pushing its scanned image. The agent can
download that artifact from the chosen successful build and use it to prepare a
reviewable release:

```sh
neckbeard release pin -env dev -image "$(cat built-image.txt)" \
  -health-url https://dev.example.com/healthz
```

The generated builds target linux/amd64 without provenance manifests, matching
the catalog's x86 runtime and the digest reported by its running containers.

HTTP releases require an HTTPS health endpoint. Optional version, environment,
and database assertions can verify the bellwether app's health response; use
`-expected-version`, `-check-environment`, and `-require-database` when preparing
that test application's release. Workers and cron definitions are inspected
through native APIs; executing cron and proving worker effects remain live
harness work.

The release runner signs into cloud registries using short-lived credentials
kept in a temporary private auth file. Promotion copies the verified source
digest without rebuilding. An already-staged digest can be used after its
original source registry is removed. Across separate cloud accounts or projects,
the target deployment identity still needs read access to the source registry;
the operator must establish that scoped grant before promotion.

Successful deployments retain a current and previous verified receipt. CI stores
the pair as one object in the environment's existing versioned infrastructure
state storage. Artifact retention does not determine rollback availability.
Storage access errors block deployment instead of being treated as a first run.
Use `restore-receipts` before preparing promotion or rollback on a fresh checkout.
CI serializes releases and infrastructure changes by environment. Do not run
concurrent manual deployments against that same environment.

```sh
neckbeard release restore-receipts -env dev
neckbeard release promote -from dev -env stg \
  -health-url https://stg.example.com/healthz

neckbeard release restore-receipts -env stg
neckbeard release rollback -env stg
```

Each command above prepares local intent; review and commit it before triggering
the release job. GitHub uses the release workflow's environment selector. GitLab
uses a main-branch pipeline with `NECKBEARD_RELEASE_ENV` set to the selected
environment, then its manual release job. Production requires the configured
repository approval gate. `configure-ci -env prd` checks the repository before publishing production
configuration: GitHub must have required reviewers with self-review disabled;
GitLab must have a protected `prd` environment with required approvals.
Administrators must preserve these rules afterward; setup does not replace the
platform's live gate or prove that an eligible independent approver is available.

## Kubernetes approval boundary

Flux remains the workload writer. Each environment has a separate application
GitRepository source that starts at an invalid commit. Its
`kustomize.toolkit.fluxcd.io/ssa: IfNotPresent` policy lets the parent bootstrap
create the object without overwriting subsequent CI-owned commit pins. The
approved release job patches this source to its exact checkout commit before
reconciling workloads. Merging a promotion into main does not advance this source.
See the [Flux apply-policy documentation](https://fluxcd.io/flux/components/kustomize/kustomizations/).

Bootstrap must use the repository's SSH deployment credential in the
`flux-system` secret, which the application source also references. The cluster
bootstrap and deployment-identity access grants still need to be completed and
tested. The generated top-level environment kustomization includes the Flux
system, controllers, application source, and application Kustomization; it does
not apply the placeholder application base directly.

## Catalog 0.3.0 migration notes

Re-plan before scaffolding; the blueprint pins the full catalog digest. Review
generated differences and preserve the ownership contract.

- Bootstrap changes the OIDC subject format. GitHub uses repo, environment,
  and ref claims; GitLab additionally binds protected-ref, deployment-tier, and
  protected-environment claims. Apply the matching bootstrap trust changes and
  repository subject customization together before using the new pipelines.
  GitHub PR plan jobs use `<env>-plan` environments. Main-branch deploy jobs use
  `<env>`. The plan identity no longer has state-write access; PR plans disable
  state locking.
- GitHub now issues immutable repository subjects for new repositories. AWS and
  Azure bootstrap accept `github_subject_prefix`, taken from the repository OIDC
  API; the value must identify the exact repository and cannot contain wildcards.
  `configure-ci` refuses a prefix mismatch before publishing variables. This was
  verified against a real GitHub token; do not infer its subject from the repo name.
- AWS's optional existing OIDC provider ARN allows later environments in the
  same account to reuse the first bootstrap's provider. A moved block preserves
  the original provider's state address when its new conditional count is used.
  Keep one owning state for each account-level provider.
- `configure-ci` now writes a non-secret `<env>.store.json` alongside deployment
  targets. Commit it with backend configuration. Apply the updated bootstrap
  outputs first so this command can resolve the receipt storage location.
- Serverless roots require the first real `app_image`, supplied by
  `releases/<env>.tfvars.json`. Runtime delivery owns subsequent image updates;
  infrastructure configuration changes still need a release to activate the new
  runtime revision. Foundation setup must precede the first image build;
  database and secret setup must precede the first full infrastructure apply.
- AWS ECS service and EventBridge task targets retain their Terraform addresses.
  Native releases select new task revisions, with ECS rollback detection and
  running-task digest checks. Worker services keep at least one running task.
- AWS/GCP ingress modules add optional HTTPS inputs and resources without moving
  existing resource addresses. Existing HTTP configurations are not silently
  assigned a hostname or certificate.
- Azure Container Apps use per-service user-assigned identities for Key Vault
  access before runtime creation. Existing app/job role-assignment addresses are
  preserved, but their principals change; review those replacements.
- All three PostgreSQL modules add opt-in application credentials through
  ephemeral, write-only password inputs. Existing direct module consumers retain
  their previous default. Generated roots opt in. Existing Azure Entra-only
  databases need an explicit credential migration before applying these roots;
  do not apply a null password to an existing server. Existing AWS databases
  require an explicit migration from their RDS-managed credential and username;
  generated roots use `neckbeard` and do not enable automatic password rotation.
- Provisioned PostgreSQL now binds a connection secret on all clouds, defaulting to
  `DATABASE_URL` or the need's explicit `secret_name`. Re-plan old blueprints.
  Grant the setup operator read/add-version access on that connection secret.
  CI also needs the cloud-specific runtime-access prerequisites in the generated
  bootstrap guide: GCP Secret Manager Admin scoped to declared application secrets
  for secret IAM bindings, or Azure Key Vault Secrets User on the application vault
  for database reads. The PR plan identity needs no secret-value access.
- GCP bootstrap refuses prefixes longer than 25 characters, which previously
  truncated the plan and apply service accounts to the same ID. Shorten the
  organization/application name for those previously undeployable configurations.
- The planner raises AWS availability-zone counts to two when required by ALB,
  RDS, or EKS, and sets a one-vCPU Cloud Run floor for this deployment topology.
  These sizing changes are recorded as derived inputs and affect estimates.
- EKS now targets Kubernetes 1.35, verified under standard support in us-east-2
  on September 9, 2026. Kubernetes 1.33 has entered extended support. Existing
  1.33 clusters must upgrade to 1.34 first, then 1.35; EKS cannot skip a minor
  version. Kubernetes deployment clients are pinned to 1.35.8. Verify regional
  support again before applying.
- Existing Kubernetes app Kustomizations must be moved to their new release
  source before relying on CI approval gates. Until that migration is applied,
  an older source following main can still deploy automatically.

## Remaining acceptance work

Complete the remaining live database verification and application secret population,
Kubernetes bootstrap/access/HTTPS, and the live twelve-combination harness.
Validate production gates, cron execution, worker effects, partial failures,
promotion, rollback, and teardown using generated paths. Local tests and provider
validation are evidence of implementation checks, not evidence of deployment.

GitLab account verification is complete. The real identity preflight passed dev
and staging; production was blocked with one pending Maintainer approval. Both
starting the blocked job and approving one's own deployment were denied. A second test used a temporary project bot to initiate the preflight and the
signed-in Maintainer to approve it: unapproved execution and bot self-approval
were denied, separate-account approval was accepted, and production preflight
job `16407719170` passed. The token was revoked afterward. This proves the
platform rule between accounts, not independent human review; the rule was not weakened.
The dedicated `neckbeard.beale.sh` DNS delegation is also in place. AWS's test
bootstrap grant was explicitly approved and its 13-resource apply succeeded.
GCP's isolated test bootstrap also succeeded with 14 additions and no changes or
deletions. AWS and GCP foundation and database setup completed through the shared
runner. Azure foundation and database setup also completed. AWS
application infrastructure passed in generated GitHub run `34412937032`; its HTTPS
health response reports the expected version/environment and `db=ok`. A created
record was processed by the worker. GCP/Azure application connectivity is still
unexercised. AWS live testing
found and fixed CLI stdin handling and the provider's write-only password conflict.
AWS CLI task configuration uses a temporary mode-0600 file because its JSON input
reader opens files twice; database secret values still use stdin.

The explicitly approved Azure bootstrap initially partially applied. Its storage create
failed because the provider attempted key authentication against an Entra-only
account. Generated providers now use Entra storage authentication, and container
creation waits for the operator's data-plane role. Generated recovery succeeded
from retained state, including the flexible federated credential. All three clouds
now have prepared private databases; passwords and connection strings were checked
in memory and are absent from their infrastructure state. Kubernetes workloads
receive their environment from a generated ConfigMap, including workers and cron jobs.


The repeatable bellwether application probe creates one record, waits for worker
processing, and checks that exact record again after an update or rollback:

```sh
python3 bellwether/probe.py --release /path/to/fixture/releases/dev.json \
  --note /path/to/evidence/dev-note.json --create
# After subsequent releases, use the same note file and omit --create.
python3 bellwether/probe.py --release /path/to/fixture/releases/dev.json \
  --note /path/to/evidence/dev-note.json
```

`bellwether/aws_cron_probe.py --root /path/to/fixture` executes the task definition
referenced by the generated EventBridge target and checks its actual image,
exit status, and report from the populated database. It does not prove the
scheduled EventBridge invocation or its IAM permissions. These probes contribute
evidence; they do not yet orchestrate the complete twelve-combination harness.


GitHub rejected required reviewers on the private test repository with HTTP 422
because its billing plan does not support that protection rule. The new
production configuration guard also refused that live repository before any
production variables were written. GitHub's current plan restriction requires
an Enterprise-enabled private repository or an approved public test fixture for
this acceptance check. See [GitHub's deployment review documentation](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/review-deployments).


AWS generated release `34413406821` passed initial native deployment verification
and durable receipt storage. Update release `34414097071` passed with version
0.2.0, and the original database record survived. A direct invocation of the
scheduled report's task definition used the expected digest and reported one
processed record. Rollback release `34414731342` restored 0.1.0 from cloud
receipts, preserving the original record and processing a new one.
Local regression tests also caught and fixed recovery after an unverified update:
rollback now selects the most recent successful receipt, rather than skipping
it for older history or failing when older history does not exist. Live failure run `34418049045` then proved the verified receipt stayed
byte-for-byte unchanged. Recovery run `34419018017` restored 0.1.0, retained the
original record and processed a fresh one.

`bellwether/gitlab_gate_probe.py` makes the GitLab two-account preflight repeatable
for a prepared private test project. It creates a one-day project-scoped token,
keeps the token in memory, and revokes it on completion or normal failure. The
evidence file records its non-secret ID for cleanup after a forcibly killed run.

Azure/GitHub/serverless recovery run `34417965294` (attempt 3) passed after
fixing drift from Azure's automatically assigned infrastructure resource-group
name. The recreated app retained its original database record. Direct scheduled
job execution `nbt260909-azghsc-dev-nightly-rep-uy96c2l` passed with the pinned
image and a populated database report. `bellwether/azure_cron_probe.py` repeats
that check; it verifies direct execution, not scheduled timer delivery. A refreshed
provider plan returned exit code 0 with no infrastructure changes.

GitHub infrastructure and release jobs now set `queue: max` on their shared
environment concurrency group. The default single waiting job canceled Azure
rollback `34419576390` when a newer infrastructure job joined the group; the
replacement rollback uses the larger queue. GitHub supports up to 100 waiting
jobs, with one running writer: see [concurrency documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#concurrency).
Actionlint 1.7.12 does not yet recognize this supported key ([upstream issue](https://github.com/rhysd/actionlint/issues/680));
validation excludes only its exact unexpected-queue-key diagnostic, retaining
all other lint checks. GitHub accepted and started the generated workflow.

Azure update release `34419295836` passed at 0.2.0 with the original record
retained and a fresh record processed. Rollback `34419784813` passed at 0.1.0;
both prior records survived and its worker processed a third record. Both AWS
and Azure latest release and infrastructure workflows finished successfully.
Promotion, Kubernetes, the remaining cloud/VCS lanes, and the complete automatic
twelve-combination harness remain unfinished. Temporary cloud resources remain
live for continuation; teardown has not run.
