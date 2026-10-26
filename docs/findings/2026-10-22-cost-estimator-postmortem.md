# Cost estimator and GCP trial postmortem — 2026-10-22

October 26 follow-up: [model v3 and the six-lane pricing matrix](2026-10-26-cost-model-matrix.md)
extend the component corrections to AWS, Azure, EKS, AKS and GKE, with live
USD/EUR price snapshots and offline regression tests. This addresses numerical
interpretation and workload modeling; it does not replace deployment or invoice
validation. The incident record below retains the findings as observed.


October 23 follow-up: [cost model v2](../../skills/neckbeard/references/cost-model.md)
implements GCP component corrections, effective Terraform inputs, strict numeric
and component coverage checks, and input-bound pre-scaffold approvals. The Umami
HCL/plan reconciliation now agrees at $87.24/month under the documented explicit
usage assumptions. The original withdrawn figures below remain incident evidence;
this is not a measured monthly bill or numerical validation of the other clouds.


Status: investigation and proposed remediation; estimator behavior is unchanged.
Scope: the Umami trial, the shared estimator/report/ceiling implementation,
all-cloud coverage risks, and the deployment process that let us report an
unreconciled number. This is not a verified numerical audit of every cloud lane.

## Executive finding

We quoted $25.57/month expected dev, then called $40.63/month a corrected
baseline. **Neither is an acceptable complete forecast.** The first priced a
different topology and incomplete usage. The second introduced a fictitious
database replica, retained a misclassified IP charge, and priced active Cloud
Run compute at zero. Neither number is an observed monthly bill.

The failure is an unchecked chain from deployment inputs through adapter
semantics, usage coverage, reporting, and operator communication. Green static
checks and successful deployment did not establish pricing correctness.

The operator should have reconciled the second breakdown before presenting it
as authoritative. The product should make that reconciliation a requirement,
not depend on an agent spotting suspicious line items.

## Reproduction and dollar reconciliation

Seven isolated pricing runs used Infracost 0.10.45 and the same pricing API on
October 22. HCL copies contain only generated modules and environment HCL;
HTTPS adds the existing public-hostname variable file. Plan tests use the saved,
password-redacted dev plan. No cloud resources were changed by these tests.
Usage is either zero or the same two million requests/10 GB backup assumptions.
These reduced usage files reproduce the original HTTP headline exactly.

| Pricing input | Baseline USD/month | Expected USD/month |
|---|---:|---:|
| Rendered HTTP HCL | 23.965 | 25.565 |
| Same HCL with hostname/HTTPS | 31.265 | 32.865 |
| Saved HTTPS plan | 40.630 | 41.430 |
| Diagnostic plan with empty replica arrays omitted | 31.265 | Not run |

Three decimals here preserve the forensic reconciliation; they are not a claim
of forecast precision. The original CLI formatted 25.565 as $25.57.

The baseline difference is exactly **$7.300 for the additional modeled HTTPS
rule plus $9.365 for the nonexistent replica**. Removing only the empty replica
configuration fields from a diagnostic JSON copy eliminates that $9.365.
This diagnoses the adapter; deleting fields from production plans is not a fix.

The original expected figure also included $0.80 backup usage and $0.80 request
charges. The quoted $40.63 plan run omitted the usage file entirely. Even when
the expected usage file is supplied, the plan prices Cloud Run compute at zero.
Comparing expected usage on one side with a partial baseline on the other was
not an apples-to-apples measurement of estimation error.

The compact [reproduction evidence](evidence/2026-10-22-cost-reconciliation.json)
records the outputs and component quantities. Full local diagnostic inputs and
outputs are retained outside Git; cloud plans, credentials and state are not
published with this postmortem.

## Confirmed causes

### 1. Effective deployment inputs are outside the estimate

`core/estimate/estimate.go` renders a fresh blueprint into scratch. It does not
read the environment's user-owned variable files or custom extensions. The
renderer declares `public_hostname` with a null default; the ingress module
creates TLS resources only when that input is populated. Our trial supplied
its hostname later through `hostname.auto.tfvars.json`.

Consequently, rerunning the ordinary estimator after adding the hostname still
priced HTTP. The blueprint hash and cost sidecar continued to match, although
the deployed topology no longer matched the estimate. This also affects AWS's
user-supplied hostname/certificate inputs; its dollar impact needs separate
reproduction. Arbitrarily copying every workspace file into the estimator is
not the proposed remedy: nonsecret cost-affecting deployment inputs must be
explicit and part of the estimate's identity.

### 2. A plan is not automatically a better pricing model

The [0.10.45 SQL Terraform adapter](https://github.com/infracost/infracost/blob/v0.10.45/internal/providers/terraform/google/sql_database_instance.go)
passes `replica_configuration` as a string. Its
[resource model](https://github.com/infracost/infracost/blob/v0.10.45/internal/resources/google/sql_database_instance.go)
adds a paid Replica whenever that string is nonempty. The provider-populated
empty array in the plan becomes a nonempty string. The saved plan explicitly
says `ZONAL`; it contains no separate read-replica resource.

Provider defaults, empty blocks and unknowns can change adapter behavior even
when HCL and plan describe the same intended service. Resolution must be
compared at component level before replacing the earlier estimate.

### 3. The networking model is wrong for this topology

The adapter charges $7.30/month for an unused public IP. Its
[address reference logic](https://github.com/infracost/infracost/blob/v0.10.45/internal/providers/terraform/google/compute_address.go)
recognizes VM attachment, not this forwarding-rule attachment. Google's
[network pricing](https://cloud.google.com/vpc/network-pricing) says an external
static IP assigned to a forwarding rule has no separate IP charge. Reservation
before attachment can still incur transient charges.

The [forwarding-rule model](https://github.com/infracost/infracost/blob/v0.10.45/internal/resources/google/compute_forwarding_rule.go)
uses the additional-rule SKU for every rule. Google's
[load-balancing pricing](https://cloud.google.com/load-balancing/pricing) groups
the first five global rules at $0.025/hour per project, then charges $0.01/hour
per additional rule. The trial's two rules therefore require service/project
aggregation, not two independent additional-rule prices. At 730 hours that
first bracket is $18.25; this is a component cross-check, not a new app total.

There are also null-quantity proxy/network components in the output. We must
validate their applicability and allocation rather than blindly adding every
adapter line and risking double-counting.

### 4. Runtime usage coverage is materially incomplete

The shared schema permits request counts but no request duration, effective
concurrency or active-instance hours. The GCP mapping supplies only request
count. The [Cloud Run model](https://github.com/infracost/infracost/blob/v0.10.45/internal/resources/google/cloud_run_v2_service.go)
needs additional inputs to calculate compute.

HCL assumes request-based behavior and charges $0.80 for requests while CPU and
memory quantities remain unset. The provider-populated plan takes a different
billing path and prices CPU and memory at zero using the zero-instance floor.
A read-only check of the actual service confirmed `cpu-throttling: false`:
instance-based billing. Google's [billing settings](https://docs.cloud.google.com/run/docs/configuring/billing-settings)
describe charges over an instance's lifetime in that mode. A zero minimum does
not make active instance hours free. We have not measured a representative
active-instance-hour workload or reconciled billing export.

`AppliedUsage` checks that a resource type exists, not that the adapter accepted
the usage field or all cost dimensions. A service can therefore appear covered
while most of its cost is absent. Type-wide assumptions also apply to every
matching resource; the report does not clearly distinguish app-wide totals
from per-service/per-resource quantities.

### 5. Unknown and unpriced components become reassuring totals

`parseBreakdown` discards numeric parse errors and treats null resource totals
as zero. It does not validate required components, quantities, finiteness or
resource coverage. It groups upstream no-price classifications under
“free / no price,” though the application cannot infer that every unpriced
service component is free. A valid empty JSON object can produce a zero total.

The report tells users to treat its high scenario as a buffer for unmapped
usage. Doubling mapped usage cannot bound costs that are never modeled. The
footer claims provisioned prices are firm; wrong resource/SKU/quantity
selection makes that claim unjustified even with current unit prices.

### 6. The ceiling is not a deployment spending guarantee

The sidecar records blueprint hash, currency, expected totals, timestamp and
adapter version. `CheckCeiling` checks a matching blueprint and expected total
before scaffold. It does not bind later variable files/custom changes, verify
coverage, enforce freshness, or reprice the authenticated plan before apply.
Generated release/infra workflows do not perform that cost comparison.

The UI and skill wording calling this “the cap” overstates the implementation.
It is currently a pre-scaffold estimate policy check. It is not an account-wide
spend cap, a complete forecast, or an enforcement mechanism over later changes.

## Cross-cloud risk review

These are code-reviewed gaps and required validation, not asserted numerical
errors in every lane:

| Surface | Confirmed shared risk | Validation still required |
|---|---|---|
| AWS serverless | No named task-running-hours assumption in the usage map; later TLS inputs are outside scratch | ECS floors/running hours, autoscaling, ALB/LCU, endpoints/NAT, storage/registry/backups |
| GCP serverless | Reproduced topology, replica, networking and compute failures | CPU billing modes, active hours, request workloads, storage/logs/secrets/builds, measured billing |
| Azure serverless | No named Container Apps runtime-hours/active-idle assumptions in the usage map | Active/idle compute, minimum replicas, free allocation policy, gateways/NAT, storage/logs/backups |
| Kubernetes on all three | Usage map does not constitute a complete capacity model | Control plane, node floor/growth, disks, spot/regular mix, ingress, egress and observability |
| Every environment/tier | All environments are summed whether or not deployed; type-wide usage scope and scenario semantics are ambiguous | Dev/stg/prd differences, all four tiers, multiple services/DBs, shared project/account cost allocation |

Existing estimator tests cover usage-file construction, type matching, report
phrasing and ceiling comparisons. They do not verify provider-normalized plan
parity or vendor billing semantics. Passing six-lane V0 validates generated
infrastructure syntax/policy, not estimated dollars. Duplicating the pricing
matrix for GitHub and GitLab is unnecessary; equivalent topology/inputs should
have the same result regardless of CI provider.

## Deployment/process review beyond the estimator

- Organization-restricted IAM required a catalog change; the HTTP startup
  probe needed a realistic cold-start allowance. Those regressions are covered
  in PR #43. Their discovery reinforces that V0 is not a runtime compatibility
  guarantee, and organization constraints need explicit preflight evidence.
- The trial required explicit app initialization, admin rotation and an
  instance-specific database CA. Successful operator assistance is useful
  evidence, but is not proof of a self-service onboarding path.
- We began with no scheduled GitHub runs, used Cloud Build, then delayed GitHub
  infra execution competed with the operator and applied an older image.
  The scheduling cause remains unconfirmed. A fallback needs an explicit
  single-writer handoff before any manual apply; absence of runs is not a lock.
- State-lock/stale-plan protection worked. The recovery did not justify
  disabling it. Autoapply must stay off until first-deploy prerequisites are
  complete, then resume from the reviewed image/configuration.
- The final Docker stage discarded upstream dependency security overrides.
  Blocking scans caught three HIGH vulnerabilities; runtime packaging and
  package-manager fixes were needed before an image could be published.
- Multiple packaging retries and the CA overlay consumed build time/cost not
  represented in the steady-state estimate. Bootstrap, initialization jobs,
  retained images/source archives and failed-run leftovers need their own
  inventory and one-time/retained-cost accounting.
- The heartbeat was liveness-only. The separate real analytics write/read and
  revision-persistence checks correctly supplied database evidence. Keep these
  assertions distinct in reports and receipts.
- Image vulnerability gates, private admin rotation, restricted ingress,
  secret injection and eventual generated WIF release verification worked.
  Do not weaken those gates to make the next trial look smoother.
- This remains one maintainer-operated HTTP/Postgres trial. Restore, rollback,
  teardown and independent-user onboarding were not established.

## Proposed estimator contract for the workshop

Keep price lookup behind an adapter; Neckbeard owns the effective inputs,
service-level cost model, coverage ledger and policy decision. Evaluate a
supported adapter upgrade against these fixtures before deciding whether to
upgrade or patch specific resource interpretations. Pinning an old version
makes its behavior repeatable; it does not make that behavior correct. A
wholesale pricing engine rewrite is not the first remediation.

Each resource/service component should be classified as priced fixed cost,
priced usage with explicit units/assumptions, verified zero/not applicable,
shared cost with allocation scope, or unknown with a reason. Null quantities
must remain unknown. Publish modeled subtotal and missing categories; publish
an expected total only when its required dimensions are modeled.

Use the same nonsecret deployment inputs for the early estimate and pre-apply
reconciliation. Record their digest along with topology, environment selection,
usage/units, catalog/adapter version and pricing provenance. Keep an early
credential-free path; require a reviewed component diff when a plan refines it.
Never store credentials or whole sensitive plans as estimate provenance.

Ranges should vary defined workload/capacity inputs. Their explanation must say
which uncertainties they cover. Unknown components have no claimed upper bound.
Separate recurring fixed cost, expected usage, shared costs, one-time setup and
retained trial resources. State which environments are planned versus deployed.

## Work packages and acceptance criteria

| Priority | Deliverable | Evidence required to close |
|---|---|---|
| P0, before trusting beta cost claims | Coverage-aware output and honest terminology | Missing/malformed totals fail; null usage is unknown; no unsupported “high covers it” or spend-cap claim; legacy/incomplete sidecars cannot claim a complete forecast |
| P0, before treating a ceiling as an apply guard | Effective-input identity and pre-apply reconciliation | Adding HTTPS/custom cost inputs invalidates approval; same workload on HCL/plan is reconciled; critical unknowns or unexplained material deltas block a passing cost gate |
| P0 for advertising GCP estimates | Correct GCP service/component interpretation | Empty replica array adds no replica; attached IP is free; 1/2/5/6-rule aggregation follows documented brackets; live and estimated billing modes agree; nonzero active usage changes compute cost |
| P1 across all supported cost lanes | Independent pricing/coverage fixture matrix | Three clouds × two runtimes × tier/env differences; positive controls for duration/hours/replicas/storage/network; input perturbations move the right components; unknowns never silently disappear |
| P1 for beta deployment readiness | Reproducible first-deploy and operator handoff | Start from clean project; one apply owner; explicit initialization/secret/CA prerequisites; build costs and leftovers recorded; same assertions on generated CI release |
| P1 to calibrate forecasts | Observed-cost reconciliation | Match actual billable SKU/quantities for a defined workload/window, account for billing delay/free tiers/credits, separate setup spend, compare forecast to observations without extrapolating a short trial as a monthly bill |

Dollar tolerances for live price checks should be chosen after the fixtures are
reconciled. Deterministic tests should use fixed price inputs and independently
specified quantities/billing rules; they should not merely reproduce the
adapter's current arithmetic. Time-varying price checks belong in a separate
explicit validation run.

Recommended first implementation chunk: the coverage ledger and truthful
report/sidecar semantics, with the Umami fixtures as regression inputs. Then
bind effective deployment inputs and fix GCP interpretation before offering a
new total. In parallel conceptually, inventory AWS/Azure/Kubernetes dimensions;
they remain unverified rather than being declared correct by association.

No estimator code, billing mode, cloud resources or budget policy was changed
by this investigation. No replacement monthly total is certified here.
