# neckbeard — Design (Draft 1)

> Status: **proposal — nothing here is committed to yet.** This doc exists to argue with.
> Draft 1 supersedes Draft 0; the changelog is in Appendix A.

neckbeard is a plugin for your favorite coding agent that analyzes your application and
scaffolds cloud infrastructure with explicit, tested security controls: environments,
CI/CD, GitOps, observability — with an honest cost estimate attached, and optionally a
full organization landing zone underneath.

Sibling of ponytail. The neckbeard handles your infra so you don't have to.

---

## 1. Product thesis (unchanged)

Coding agents are good at app code and bad at freehanding infrastructure. The failure
mode is always the same: plausible-looking HCL that is subtly broken, insecure by
default, and unestimable. neckbeard inverts the roles:

- **Humans (us) maintain the golden path**: tested, versioned OpenTofu modules,
  pipeline templates, and delivery layouts per cloud.
- **The agent does analysis, capability selection, parameterization, and explanation**:
  it reads the user's app, determines what it needs, maps that onto catalog inputs, and
  explains its decisions and their consequences.

Planning, rendering, and validation are deterministic code. **The agent must not invent
infrastructure outside the catalog.** Unsupported needs produce an actionable
explanation, never a force-fit (§8).

**Launch scope**: AWS, GCP, and Azure, on GitHub Actions and GitLab CI — all six
cloud/VCS combinations are first-class launch requirements. Scope is constrained by
narrowing the **workload contract** (§4) and configuration surface, not by deferring
providers.

## 2. Target users and entry paths

Two personas, two entry paths, one catalog.

### Path A — Adopt (app developer, existing cloud footprint)

The common case: a developer with an application and access to existing cloud
containers — AWS accounts, GCP projects, or Azure subscriptions — who wants
production-shaped infrastructure inside them.

- **Starting point**: one existing account/project/subscription per environment
  (dev/stg/prd), or fewer — a single shared one is accepted with a written warning
  about blast radius and IAM separation limits.
- **Required authority**: admin-equivalent IAM in each target container — enough to
  create the OIDC identity provider / workload identity federation config, IAM roles,
  networks, and the state backend. neckbeard checks this during `configure` and lists
  exactly which permissions are missing rather than failing mid-apply.
- **What neckbeard does NOT touch on this path**: org structure, billing, SCPs/org
  policies, other workloads in the account. Instead it emits a **posture report**:
  which landing-zone controls (central audit logs, guardrails, break-glass, billing
  alerts at org scope) are absent or unverifiable in this footprint, so the gap is
  explicit rather than silently assumed away.
- **Ownership**: the app team owns everything generated; state lives in a backend
  inside their own containers.

### Path F — Found (platform owner, new organization)

The differentiator case: someone with org-root authority establishing a new landing
zone — org structure, billing wiring, security/audit accounts, guardrails, environment
containers — that many applications will then consume via Path A.

- **Required authority**: AWS management-account admin, GCP Organization Admin, or
  Azure tenant Global Admin + subscription-creation rights (EA/MCA billing authority).
  This is deliberately scary; the docs say so and the bootstrap flow prints exactly
  what it will create before doing it.
- **Output**: a **platform repo** (landing zone state + config), plus per-environment
  containers ready for Path A consumers.
- **Ownership**: the platform owner owns the platform repo and Layer 0 state. App
  teams never hold Layer 0 credentials.

### How applications reuse shared infrastructure

The platform repo publishes an **environment manifest** per environment: a versioned
JSON document (account/project/subscription IDs, network IDs, DNS zone, registry,
cluster reference if any, OIDC audience config, state-backend location, allowed
regions). App repos pin a manifest reference in `neckbeard.yaml`; the planner consumes
it as input, and rendered HCL reads it via data sources at plan time.

Manifest, not cross-repo remote-state reads: remote state coupling would require app
pipelines to hold read access to platform state (over-broad) and would couple app repos
to the platform repo's internal state layout. The manifest is a deliberate, versioned
interface. **Recommended distribution**: an object-storage location created by Layer 0,
readable by env-scoped app roles. (Decision D5, §14.)

Multiple app repos per environment are the expected shape: each app gets its own
workload state, namespace/service scope, DNS subdomain, and IAM boundary inside the
shared environment containers.

## 3. Launch support matrix

### 3.1 Workload contract v1

What an application may need for neckbeard to fully support it:

| Capability | Contract |
|---|---|
| HTTP services | 1–10 containerized services, HTTP/HTTPS ingress, custom domain + TLS |
| Background workers | containerized, queue-less (poll/DB-backed), scale by CPU/mem |
| Scheduled jobs | containerized cron-style jobs |
| Database | managed **PostgreSQL** only |
| Object storage | buckets with app-scoped IAM |
| Secrets | cloud secrets manager, synced to runtime; **names/references only, never values** |
| Container images | cloud registry, built in CI, immutable digests |
| Existing external services | referenced via connection secrets (e.g., an existing DB, a SaaS API) — supported as references, never provisioned |

### 3.2 Explicitly unsupported at launch

Each produces a named explanation (§8), not a nearest-fit: other database engines
(MySQL, MongoDB, etc.), caches (Redis/Memcached), queues and streams (SQS, Pub/Sub,
Service Bus, Kafka), functions-as-a-service, VMs, GPU/ML serving, stateful workloads on
Kubernetes (operators, StatefulSets beyond the optional o11y bundle), service mesh,
multi-region active-active, Windows containers, on-prem/hybrid connectivity, non-HTTP
ingress (raw TCP/UDP).

### 3.3 Equivalent outcomes across clouds

The contract defines **outcomes**; each cloud satisfies them with its native services.
Resource graphs are *not* identical, and meaningful differences are surfaced in `plan`
output rather than hidden:

| Outcome | AWS | GCP | Azure | Meaningful differences surfaced |
|---|---|---|---|---|
| Runtime: kubernetes | EKS | GKE | AKS | node pricing models, upgrade cadence, LB integration |
| Runtime: serverless-containers | ECS on Fargate | Cloud Run | Container Apps | Cloud Run & Container Apps scale to zero; ECS services have a min task count (idle floor cost); Container Apps scaling is KEDA-based |
| PostgreSQL | RDS for PostgreSQL | Cloud SQL for PostgreSQL | Azure Database for PostgreSQL – Flexible Server | HA models differ (Multi-AZ vs regional vs zone-redundant); maintenance/failover semantics differ |
| Object storage | S3 | GCS | Blob Storage | consistency and class/lifecycle models |
| Secrets | Secrets Manager | Secret Manager | Key Vault | per-secret vs per-vault pricing and access models |
| Registry | ECR | Artifact Registry | ACR | |
| Ingress + TLS | ALB + ACM | External Application LB + managed certs | Application Gateway / Container Apps ingress | cert automation and WAF options differ |
| DNS | Route 53 | Cloud DNS | Azure DNS | |
| CI → cloud auth | GitHub/GitLab OIDC → IAM role | → Workload Identity Federation | → Workload identity federation | no static cloud keys on any lane ([GitHub OIDC docs](https://docs.github.com/en/actions/security-for-github-actions/security-hardening-your-deployments/about-security-hardening-with-openid-connect), [GitLab id_tokens docs](https://docs.gitlab.com/ci/cloud_services/)) |

### 3.4 The combination lattice

First-class launch lanes: **3 clouds × 2 VCS = 6 lanes**, each supporting **2
runtimes** (kubernetes, serverless-containers) = 12 tested paths. Everything else is
pinned to keep that lattice testable: one database engine, one region per deployment,
a fixed dev/stg/prd environment set, Flux as the only GitOps tool, one delivery
topology per runtime. Widening any pinned dimension is a post-launch decision with its
own test cost.

## 4. Runtimes and tiers

### 4.1 Two launch runtimes, consistent with our own advice

Draft 0 recommended serverless containers for small teams and then cut them from the
MVP; that contradiction is resolved: **both runtimes ship at launch**, and the vertical
slice (§13) is built serverless-first because it is the recommendation for the smallest
users. Testing implication accepted: both runtimes are in the release matrix (§12.3).

**Runtime is orthogonal to tier.** Kubernetes is recommended when the team already
operates it, runs several services with shared cluster concerns, or needs in-cluster
capabilities; serverless-containers otherwise. Company growth does not imply
Kubernetes, and neckbeard never auto-migrates a runtime.

### 4.2 Tiers are presets over explicit dimensions

Tier names alone decide nothing. Each tier is a documented preset over four visible
dimensions, every one individually adjustable within supported ranges:

| Dimension | What it drives |
|---|---|
| traffic band (req/s, data volume) | sizing, autoscaling bounds, NAT strategy |
| availability objective | multi-AZ/zone redundancy, replica count, LB topology |
| recovery objectives (RPO/RTO) | backup cadence, PITR, snapshot retention, restore drills |
| operating capacity (who's on call) | self-hosted vs managed choices, alert routing defaults |

Presets: `solo`, `smallteam`, `established`, `regulated` (names TBD — Decision D6).
Nothing in a preset assumes growth: `established` does not force read replicas or
Kubernetes; a replica appears when the availability/RPO dimensions call for it.

**Consequences before generation**: `plan` prints a topology summary (what will exist,
per env) and `estimate` prints the cost consequences; `scaffold` warns if the blueprint
changed since the last estimate. Nothing is generated before the user has seen both.

## 5. Reference architecture

Two OpenTofu layers with separate state boundaries and separate owners.

### Layer 0 — Landing zone (Path F only; Path A gets a posture report instead)

| Concern | AWS | GCP | Azure |
|---|---|---|---|
| Org container | Organizations + OUs | Organization + folders | Management groups |
| Billing | management account, consolidated billing | billing-admin project | billing scope on MG |
| Security/central | security OU: audit + log-archive accounts | logging + security projects | platform landing zone: management + connectivity subscriptions |
| Environments | account per env | project per env | subscription per env |
| Guardrails | SCPs | org policies | Azure Policy |

Layer 0 state lives in a dedicated bootstrap backend (§10.3).

### Layer 1 — Workload (per application, per environment)

Rendered per env from the catalog: network, runtime (cluster or serverless service
platform), PostgreSQL, buckets, secrets, registry, DNS + TLS, observability wiring.
Each environment is an **independent state** with its own OIDC-scoped plan/apply
identities.

### Environment isolation: no shared clusters at launch

Draft 0 floated dev/stg sharing a cluster while claiming per-env accounts and
independent states — those conflict (a shared cluster must live in *some* account and
*some* state that other envs then depend on). Resolution: **at launch, every
environment is fully isolated** — its own container, network, runtime, and state. No
resource is owned by one env's state and consumed by another's.

Cost consequence: a dedicated dev EKS/GKE/AKS cluster is real money (control-plane fee
plus nodes). Mitigations in the dev preset: smallest supported node shapes,
spot/preemptible node pools, scale-down defaults — and the estimate shows the number so
users can choose serverless-containers for a cheap dev tier consciously, accepting the
runtime-fidelity tradeoff, which `plan` states. Shared non-prd clusters are a
post-launch feature that requires a designated owning state and a published interface,
same pattern as the environment manifest — designed then, not implied now.

Shared **landing-zone** resources (log-archive, DNS root zone, registry if
centralized) are owned by Layer 0 state and consumed by app repos only through the
environment manifest — never by direct cross-state reference.

## 6. Ownership, regeneration, and stable identity

### 6.1 File ownership zones

Every scaffold writes a manifest (`.neckbeard/manifest.json`) recording each generated
file's path, the blueprint hash, and the file's content hash. Three zones:

- **Generated (neckbeard-owned)**: `infra/`, `clusters/`, pipeline files, runbooks.
  Carry a `# generated by neckbeard — edit via overrides or in designated blocks`
  header. Regenerated freely **only if unmodified**.
- **User-owned**: `neckbeard.yaml`, app code, and designated extension points
  (`infra/envs/*/custom.tf`, a custom pipeline-step include). Never written after
  first creation.
- **Reviewed inputs**: `app-profile.yaml` (agent-written, user-corrected; regeneration
  proposes a diff, never clobbers confirmations).

### 6.2 Regeneration contract

Re-running `scaffold` is predictable: for each generated file, if its current hash
matches the manifest, it is replaced; if it was hand-edited, neckbeard **never silently
overwrites** — it writes the new version alongside as `<file>.neckbeard-new`, prints a
diff, and exits nonzero until the user resolves (accept, keep, or move the edit into an
extension point or override). Same-inputs regeneration is byte-identical (§9).

### 6.3 Stable resource identity and the module-wrapping caveat

Resource names follow `{org}-{app}-{env}-{component}` with documented length/character
adaptations per cloud. The catalog is semver'd with a **resource-address stability
contract**: within a major version, module input schemas and resource addresses don't
break; a major version may move addresses and must ship `moved` blocks and/or a written
state-migration procedure.

Before 1.0, minor releases may change planner defaults with explicit migration
notes and re-planning (for example, 0.2.0 preserves image CMD defaults). This
pre-stability allowance does not relax the schema or resource-address contract.

Honesty about wrapping: putting community modules (terraform-aws-modules et al.) behind
our input schema stabilizes the *interface*, *not* the internals — swapping a wrapped
module's implementation changes resource addresses and is a **major** catalog version
with a migration path, period. Automated `neckbeard upgrade` comes later, but pinning,
identity, and migration semantics are contractual now so later automation has something
to stand on.

## 7. Internal architecture

```
                 ┌───────────────────────────────────────────────┐
  repo ──────────▶  ANALYZER   (agent) app inspection → app-profile
                 └──────┬────────────────────────────────────────┘
                        ▼
  neckbeard.yaml ─▶  PLANNER    (deterministic) profile + config +
  env manifest ──▶              tier presets + catalog@pinned
                        │       → blueprint (resolved, pinned, diffable)
            ┌───────────┼───────────────┐
            ▼           ▼               ▼
        ESTIMATOR    RENDERER        DOCS
        temp HCL →   blueprint →     runbooks, posture
        Infracost    infra/, ci/,    report, cost report
        (+plan JSON  clusters/
         if creds)      │
                        ▼
                    VALIDATOR   V0 static · V1 authenticated plan ·
                                V2 deployment verification (§12)
```

The agent's judgment is concentrated in ANALYZER and in explaining PLANNER/ESTIMATOR
output. PLANNER, RENDERER, and VALIDATOR are deterministic code — same inputs, same
bytes. The core is a standalone CLI the plugin shells out to, keeping it agent-agnostic
(Decision D1 recommends Go).

## 8. Analysis and the app profile

`app-profile.yaml` separates epistemic categories; every downstream consumer knows
which is which:

- **Detected facts** — with evidence references (`Dockerfile:14`, `mix.exs:32`,
  `DATABASE_URL read in config/runtime.exs:8`).
- **Inferences** — with stated confidence and reasoning ("worker: found Oban config →
  background jobs run in-process against Postgres; no separate queue needed").
- **Assumptions** — defaults the user hasn't confirmed, flagged loudly.
- **User-confirmed requirements** — answers to questions inspection cannot settle:
  expected traffic, availability expectations, RPO/RTO, compliance constraints, and
  **which detected dependencies already exist as managed services**. Finding a Postgres
  client never auto-provisions a database; it generates the question "provision new, or
  reference existing via connection secret?"

Secrets are recorded as **names and reference paths only — never values**, and the
analyzer is prohibited from reading value-bearing files (`.env` contents beyond key
names, mounted credential files) into the profile.

**Unsupported workloads**: analysis that detects an out-of-contract need (Redis, Kafka,
GPU) emits a named finding with what was detected, why it's out of contract, the
nearest supported alternative *if honestly adequate*, and a pointer to the roadmap. It
never silently maps Redis onto "more Postgres."

## 9. The blueprint contract

Authority chain, strictly one-directional:

```
app-profile.yaml + neckbeard.yaml + environment manifest
        │  (planner, with catalog @ pinned version)
        ▼
blueprint.yaml         ← the lockfile: every module @exact version, every input
        │                 concrete; pins catalog version, OpenTofu version,
        │                 provider versions, renderer version
        ▼
generated files        ← pure function of blueprint; byte-identical on re-render
```

- **Determinism**: same profile + config + manifest + catalog version ⇒ byte-identical
  blueprint ⇒ byte-identical rendering. Golden tests enforce this (and only this —
  they prove rendering stability, not deployability; that's §12).
- **Known vs unknowable**: the blueprint contains only decisions makeable before
  deployment. Deploy-time outputs (VPC IDs, LB hostnames, cluster endpoints) are
  explicitly *not* blueprint fields; they flow through OpenTofu outputs into the
  environment manifest after apply.
- **Overrides**: `neckbeard.yaml` overrides target only allowlisted inputs per module,
  are schema-validated, and pass the same policy checks as everything else. The
  blueprint marks each resolved input `preset | user-override(supported) |
  user-override(warned)`; values outside the tested envelope render with a warning
  recorded in the blueprint and cost report — the golden-path promise is scoped to
  `preset` and `supported` values, and the tool says so rather than pretending
  arbitrary overrides are tested.

## 10. Security: explicit controls, not adjectives

### 10.1 Control table (excerpt; full table lives with the catalog)

| Control | Mechanism | How it's tested | Known limitation |
|---|---|---|---|
| No static cloud keys in CI | OIDC/workload identity federation on all 6 lanes ([GitHub](https://docs.github.com/en/actions/security-for-github-actions/security-hardening-your-deployments/about-security-hardening-with-openid-connect), [GitLab](https://docs.gitlab.com/ci/cloud_services/)) | release matrix runs every lane with federation only | user can still add keys by hand |
| Env blast-radius isolation | separate account/project/subscription + state per env | V2 verifies cross-env access fails | Path A single-container mode weakens this; posture report says so |
| Least-privilege CI identities | per-env plan (read) and apply (write) roles, scoped to the app's resource prefix | policy tests on role docs; V2 exercises them | scoping precision varies by provider |
| Secrets never in git | cloud secrets manager + runtime sync (external-secrets on k8s; native refs on serverless) | repo-scan check in CI; V2 round-trip | pre-existing leaked secrets are out of scope |
| Private-by-default network | workloads in private subnets; ingress only via managed LB | policy tests + V2 reachability probe | serverless platforms differ in what "private" exposes; per-cloud notes |
| Supply-chain gates | image scan (trivy) + IaC policy (checkov/conftest) gate merges | pipeline execution tests | scanners miss things; gates ≠ guarantees |
| Org guardrails (Path F) | SCPs / org policies / Azure Policy starter set | policy unit tests + V2 probe (e.g., public bucket creation denied) | Path A can't install these; posture report flags |

### 10.2 Budget alerts are notifications, not caps

Scaffolded budget alerts (AWS Budgets, GCP budget alerts, Azure cost alerts) **notify;
they do not stop spending** ([AWS Budgets docs](https://docs.aws.amazon.com/cost-management/latest/userguide/budgets-managing-costs.html),
[GCP budget docs](https://cloud.google.com/billing/docs/how-to/budgets)). The cost
report and runbook say this explicitly instead of implying a spending ceiling.

### 10.3 Bootstrap credentials, state protection, partial-failure recovery

- **Bootstrap** (both paths) is the one imperative step: a human with elevated
  credentials runs `neckbeard bootstrap`, which prints its full intended resource list,
  then creates the state backend **first** — object storage with versioning +
  encryption + locking (S3+lockfile/DynamoDB, GCS, Azure Blob w/ lease) — then the CI
  OIDC trust and roles. It is idempotent: re-running after partial failure detects
  existing resources and continues or emits import instructions; it never recreates a
  backend it can see.
- **State protection**: backends are private, versioned, encrypted; access limited to
  the env's plan/apply roles and named humans. Docs state plainly that OpenTofu state
  can contain sensitive values and is protected as a secret.
- Elevated bootstrap credentials are used for bootstrap only; everything after runs on
  federated CI identities. The runbook includes a break-glass procedure and what to
  rotate if bootstrap credentials leak.

## 11. Delivery: pipeline model, promotion, approvals

### 11.1 A small shared pipeline model, because both VCSs are launch requirements

An internal model covering only the behaviors we actually generate: job graph with
dependencies, per-job OIDC identity, artifact hand-off, environment binding, manual
gates, and a user extension hook. Two renderers (GitHub Actions, GitLab CI) with
golden tests for rendering and **execution tests** (§12.3) for behavior — auth,
artifacts, promotion, deployment. Provider-specific escape hatches are explicit
passthrough blocks, not abstractions.

### 11.2 Immutable artifact promotion

Build **once**: CI builds the image, pushes to the env registry path, and records the
**digest**. Promotion moves the digest reference; nothing is ever rebuilt per
environment.

- **kubernetes runtime**: Flux per cluster; [image automation](https://fluxcd.io/flux/components/image/)
  commits new digests to the dev overlay; promotion to stg/prd is a CI-opened PR/MR
  bumping the digest in that env's overlay, merged by a human for prd.
- **serverless-containers runtime**: each env has a small, separate **release state**
  (just the service revision + digest input) so app deploys are a fast targeted apply
  that cannot touch base infrastructure; promotion is the same digest-bump PR pattern
  against a per-env release file. (Decision D3 confirms release-state vs native deploy
  CLI; release-state is the recommendation for uniformity and auditability.)

Infra changes and app deploys are decoupled: infra PRs run plan → policy → apply jobs
against `infra/envs/*`; app deploys touch only overlays/release files. A change to both
is two reviewable diffs in one PR.

### 11.3 Production approvals — tier-honest enforcement

Platform-enforced deployment gates are **paid-tier features**: GitHub environments'
*required reviewers* work on private repos only with Enterprise Cloud
([GitHub docs](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments));
GitLab *protected environments + deployment approvals* require Premium/Ultimate
([GitLab docs](https://docs.gitlab.com/ci/environments/deployment_approvals/)).

neckbeard therefore renders **two enforcement levels** and reports which one a given
repo actually has:

1. **Platform gate** (GH Enterprise / GitLab Premium+): environment-bound prd jobs
   requiring named reviewers before execution.
2. **Universal fallback** (all tiers): prd promotion happens only via PR/MR into a
   protected ref with restricted merge rights; the apply job runs only from that ref.
   Enforcement rides on protected-branch merge permissions, which every tier has.

`validate` reports the active level; the runbook states exactly what is and is not
enforced. No silent pretending that a YAML `environment:` key is an approval gate.

## 12. Validation and verification — precise claims

### 12.1 Three levels, reported honestly

| Level | What runs | What it proves | What it cannot prove |
|---|---|---|---|
| **V0 static** (offline; no cloud creds — `tofu init` downloads providers, `validate` runs unauthenticated) | `tofu fmt`/`validate`, checkov/conftest policy, kubeconform, actionlint / GitLab CI schema lint | syntax, schema, policy conformance, rendering stability | resolvability against a real account; deployability; runtime behavior |
| **V1 authenticated plan** | `tofu plan` with the user's env credentials (plan generally **requires** provider auth — data sources, refresh) | provider auth works; references resolve; many permission errors surface | apply success; quota; runtime health |
| **V2 deployment verification** | apply → health probe → change → teardown in a sandbox | the thing actually deploys, serves, updates, and cleans up | production-specific conditions |

Every `validate` run emits a report with three explicit buckets: **PASSED / FAILED /
NOT EXERCISED**. Offline runs list V1/V2 as NOT EXERCISED — never implied-passed.
Golden-file tests prove rendering determinism only and are never presented as
correctness evidence.

### 12.2 User-facing vs release-process verification

V2 automation *for users* (sandbox account vending, etc.) is a separate, later delivery
decision (Decision D4). But V2 is **mandatory in our own development and release
process from the first working milestone** — the catalog's "tested golden path" claim
is only as true as this.

### 12.3 Release matrix

A representative reference application — **bellwether** (HTTP service + worker +
scheduled job + Postgres + bucket + secrets) — runs through **all six cloud×VCS lanes,
on both runtimes (12 paths)** in release CI against real cloud accounts:

deploy → health verification → an application update via the real promotion flow → an
infrastructure change (plan/apply) → regeneration-safety check (§6.2) → teardown, with
leak detection on residual resources. Where the catalog claims a recovery property
(e.g., PITR / snapshot restore per the RPO preset), the matrix **exercises a restore**,
not just the backup setting. Ephemeral test containers per run (vended account /
project / resource group — Decision D7 covers mechanics per cloud). This matrix costs
real money and wall-clock time per release; that's the price of the claim, and a nightly
subset + per-release full run keeps it sane.

## 13. FinOps

### 13.1 Estimation pipeline (corrected)

Ordinary `tofu plan` is **not** generally credential-free, so estimation does not
depend on it:

1. **Pre-scaffold estimate (no credentials)**: the renderer materializes the blueprint
   as HCL in a scratch directory; [Infracost parses HCL directly](https://www.infracost.io/docs/features/terraform/)
   — no Terraform/OpenTofu binary, plan JSON, or cloud credentials required — pricing
   via its Cloud Pricing API. This is how `estimate` can run before anything is
   scaffolded into the user's repo.
2. **Higher resolution (credentials available)**: after V1, the authenticated plan
   JSON feeds Infracost for counts/lookups HCL parsing can't resolve.

### 13.2 Cost report structure

Per environment and total, monthly and yearly, with five distinct sections:

1. **Baseline provisioned** — resources that bill while idle (control planes, min
   task/instance floors, DB instances, NAT gateways, LBs).
2. **Usage assumptions** — explicit numbers behind every usage-based line: requests/mo,
   GB egress, NAT-processed GB, cross-zone GB, log ingest+retention GB, backup/snapshot
   GB, storage growth. Tier presets provide defaults; every number is editable in
   `neckbeard.yaml` (`finops.usage`). **A tier name is not a usage estimate; the
   numbers are.**
3. **Scenario ranges** — low / expected / high usage scenarios, not one false-precision
   figure.
4. **Shared platform costs** — landing-zone/central items (audit log storage, org
   NAT/egress patterns, centralized registry) shown separately so app teams don't
   mistake them for app costs, and Path A users see what they're *not* paying for yet.
5. **Unpriced / unsupported items** — anything Infracost or our mapping can't price is
   listed by name, never silently dropped.

**Provenance and freshness**: report footer records Infracost version, Cloud Pricing
API price snapshot date, catalog version, and blueprint hash. And per §10.2: budget
alerts notify, they don't cap.

## 14. Decisions

### Resolved in this draft

- Both runtimes at launch; serverless-first vertical slice (§4.1).
- Full env isolation; no shared clusters at launch (§5).
- Environment manifest over remote-state coupling (§2).
- Release-process V2 across all 12 paths is non-negotiable (§12).
- Estimation from rendered HCL pre-scaffold; plan JSON as refinement (§13.1).
- Tier-honest approval enforcement with universal fallback (§11.3).

### Open (tradeoff + recommendation; not silently assumed)

- **D1 Core language — RESOLVED 2026-09-08: Go.** The IaC ecosystem (HCL parsing,
  OpenTofu tooling, provider schemas) is Go-native, and the core ships as a single
  static binary the plugin shells out to. Zig was considered and rejected for
  ecosystem reasons (no mature HCL/JSON-Schema/YAML tooling); Elixir fits services,
  not distributable CLIs.
- **D2 Catalog internals.** Wrap community modules (velocity; upstream drift risk;
  §6.3 migration caveat applies regardless) vs write our own (control; slower across 3
  clouds). **Recommend wrap for launch** behind our schema, with the §6.3 contract
  making future swaps honest majors.
- **D3 Serverless deploy mechanism.** Small per-env release state (uniform, audited,
  slower) vs native deploy CLIs (fast, divergent). **Recommend release state.**
- **D4 User-facing V2 sandbox automation.** Ship post-launch; V0+V1 with honest NOT
  EXERCISED reporting is the launch posture. **Recommend post-launch.**
- **D5 Manifest distribution.** Object storage (recommended) vs a file in the platform
  repo (simpler, but couples app repos to platform-repo access).
- **D6 Tier names/count.** Placeholder `solo/smallteam/established/regulated`.
- **D7 Ephemeral test-container vending per cloud** for the release matrix (AWS account
  vending vs reuse+nuke; GCP project-per-run; Azure sub reuse + RG-per-run).
- **D8 Name collision check** (GitHub/npm/crates/pypi "neckbeard") before anything is
  published.

## 15. Milestones — vertical slice first

Acceptance criteria throughout are **deployment success, safe regeneration, a
subsequent change, understandable costs, recoverable failure** — never merely
"files generated, static checks green."

- **M0 — Contracts.** Schemas (`neckbeard.yaml`, `app-profile.yaml`, blueprint,
  environment manifest), planner core, golden determinism tests, ownership manifest +
  regeneration behavior. *Accept:* byte-identical re-plans/re-renders; hand-edit
  triggers the §6.2 refusal path.
- **M1 — Vertical slice, all six lanes.** bellwether on the **serverless-containers**
  runtime, Path A (existing containers), minimal architecture, on AWS+GCP+Azure ×
  GitHub+GitLab. Release harness runs deploy → health → digest promotion → infra change
  → regenerate → teardown on all 6 lanes. Estimate report (sections 1, 2, 5) ships.
  *Accept:* all 6 lanes green in release CI, including the update and teardown; a human
  can read the cost report and say what the expected monthly bill is and why.
- **M2 — Kubernetes runtime.** EKS/GKE/AKS + Flux + image automation + promotion PRs
  across all 6 lanes (release matrix now 12 paths); optional self-hosted o11y bundle;
  scenario cost ranges. *Accept:* 12 paths green including promotion-flow execution
  tests; o11y bundle documented per §16.
- **M3 — Founding path.** Layer 0 on all three clouds, guardrail starter sets with
  policy probes in V2, posture report for Path A, environment-manifest publication,
  multi-app-repo consumption demonstrated. *Accept:* a fresh org → landing zone → two
  app repos deploying into shared envs, on at least one cloud end-to-end in CI, org
  bootstrap runbook validated by someone who didn't write it.
- **M4 — Depth and launch.** Recovery drills wired into the matrix for claimed RPO/RTO
  presets; full cost provenance; runbooks; plugin UX pass; dogfood against a real
  internal application (Bellafonte-shaped) and fix what breaks. *Accept:* a restore
  drill passes on every cloud; dogfood app deployed and updated through neckbeard only.

Dogfooding is not a final phase: bellwether *is* dogfood from M1 onward; M4 adds a
real, messy application.

## 16. Observability defaults

**Baseline = cloud-native, on by default**: CloudWatch / Cloud Monitoring + Cloud
Logging / Azure Monitor. Rationale: zero components for the user to operate or upgrade,
pay-per-use at small scale, and the user benefit at launch is *dashboards and alerts
that exist* — golden-signal dashboards and starter alerts (error rate, p95 latency,
saturation, DB health, budget notification) provisioned from the catalog, with log
retention set explicitly per env (costs shown in the §13.2 report, where log ingestion
is a named usage assumption).

**Optional self-hosted bundle** (kubernetes runtime only, off by default):
kube-prometheus-stack + Loki + Grafana via Flux-pinned HelmReleases. Choosing it is
accepting ownership, and the docs say so concretely: the user operates and upgrades it
(neckbeard pins versions; bumps arrive as catalog updates to review), default retention
15d metrics / 30d logs, sized per tier with PVC costs in the estimate. Not recommended
below `established` operating capacity.

**Tracing is not silently half-shipped.** Default: off. If enabled, neckbeard deploys
an OTel collector *and* wires a concrete store-and-query backend — the cloud-native one
(X-Ray / Cloud Trace / Application Insights) — and the docs state plainly that the app
must emit OTLP for any of this to show anything. A collector with nowhere to send
traces, or traces with no query UI, doesn't count as tracing.

## 17. Repository layouts

**neckbeard's own repo**

```
neckbeard/
├── DESIGN.md
├── plugin/                    # Claude Code plugin (skills, commands); thin shell over core
├── core/                      # deterministic CLI: planner, renderer, estimator, validator
├── catalog/
│   ├── aws/ gcp/ azure/       # modules: network, runtime-k8s, runtime-serverless,
│   │                          #   postgres, storage, secrets, registry, dns-ingress,
│   │                          #   o11y, landing-zone
│   └── POLICY/                # conftest/checkov policy sets + tests
├── pipeline/                  # shared pipeline model + GHA/GitLab renderers
├── templates/                 # flux layouts, overlays, dashboards, runbooks
├── schemas/                   # JSON Schemas for every durable artifact
├── bellwether/                # reference app for the release matrix
├── release-harness/           # the 12-path matrix: deploy/health/update/teardown/drills
└── tests/                     # golden blueprints, golden pipelines, policy tests
```

**A user's app repo (Path A)**

```
their-app/
├── neckbeard.yaml             # user-owned config (incl. manifest ref, overrides, usage numbers)
├── app-profile.yaml           # reviewed input (facts/inferences/assumptions/confirmed)
├── blueprint.yaml             # generated lockfile
├── .neckbeard/manifest.json   # ownership + hashes
├── infra/
│   ├── envs/{dev,stg,prd}/    # generated; independent states
│   │   └── custom.tf          # user extension point, never regenerated
│   └── release/{dev,stg,prd}/ # serverless runtime: per-env release state
├── clusters/{dev,stg,prd}/    # k8s runtime: flux + kustomize overlays
├── .github/workflows/ | .gitlab-ci.yml
├── costs/estimate-<date>.md
└── docs/                      # runbooks, posture report, validation reports
```

**A platform repo (Path F)** — landing-zone states, guardrail policies, environment
manifest publication, org runbooks.

---

## Appendix A — Changes from Draft 0

1. All three clouds and both VCSs are launch requirements; scope is constrained via the
   workload contract and pinned dimensions instead (§3).
2. Two entry paths (Adopt/Found) with authority prerequisites, posture report, platform
   repo, and the environment-manifest interface for multi-app reuse (§2).
3. Serverless-containers promoted into launch scope and made the vertical-slice
   runtime, resolving the recommend-but-don't-ship contradiction; tiers reworked as
   presets over explicit dimensions with no growth⇒k8s/replicas assumptions (§4).
4. Shared dev/stg clusters removed from launch; full per-env isolation with the cost
   consequence stated (§5).
5. Validation split into V0/V1/V2 with PASSED/FAILED/NOT-EXERCISED reporting; corrected
   the credential-free-plan assumption; 12-path release matrix with deploy, health,
   update, teardown, and recovery drills made part of our release process (§12).
6. Cost architecture rebuilt: estimation from temporarily rendered HCL (Infracost HCL
   parsing, verified), plan JSON as refinement, five-section report with explicit usage
   numbers, provenance/freshness, shared-platform costs, unpriced items, and
   budgets-are-not-caps stated (§13).
7. Ownership zones, hash-manifest regeneration contract (no silent overwrites),
   naming/identity scheme, catalog semver with resource-address stability, and the
   module-wrapping migration caveat made contractual (§6, §9).
8. App profile split into facts/inferences/assumptions/confirmed with evidence refs,
   secret-name-only rule, existing-service references, and named unsupported-workload
   findings (§8).
9. Pipeline model scoped to needed behaviors with execution tests; immutable digest
   promotion specified for both runtimes; prd approvals made tier-honest with a
   universal protected-ref fallback (verified GH Enterprise / GitLab Premium gating)
   (§11).
10. Observability baseline switched to cloud-native; self-hosted bundle made an opt-in
    with explicit ownership/retention/sizing; tracing requires a real backend or stays
    off; "secure" replaced by a control/mechanism/test/limitation table; bootstrap,
    state protection, and partial-failure recovery specified (§10, §16).
11. Milestones reworked around an M1 vertical slice across all six lanes with
    deployment verification from the start; acceptance criteria defined as deploy
    success, safe regeneration, subsequent change, understandable costs, recoverable
    failure (§15).

## Appendix B — Verified references

- Infracost parses HCL without cloud credentials / Terraform binary; pricing via Cloud
  Pricing API — https://www.infracost.io/docs/features/terraform/ ,
  https://www.infracost.io/docs/cloud_pricing_api/overview/ (verified 2026-09-08)
- GitHub environments: required reviewers on private repos require GitHub Enterprise —
  https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments
  (verified 2026-09-08)
- GitLab deployment approvals / protected environments: Premium & Ultimate —
  https://docs.gitlab.com/ci/environments/deployment_approvals/ (verified 2026-09-08)
- GitLab CI OIDC via id_tokens to AWS/GCP/Azure — https://docs.gitlab.com/ci/cloud_services/
  (verified 2026-09-08)
- GitHub Actions OIDC to cloud providers —
  https://docs.github.com/en/actions/security-for-github-actions/security-hardening-your-deployments/about-security-hardening-with-openid-connect
- Flux image update automation — https://fluxcd.io/flux/components/image/
- AWS Budgets (alerts, not caps) —
  https://docs.aws.amazon.com/cost-management/latest/userguide/budgets-managing-costs.html
