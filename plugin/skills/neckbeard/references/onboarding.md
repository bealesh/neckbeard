# First application: supported path and review contract

The installed binary contains the catalog, schemas, and tier presets. No source
checkout is needed. Run `neckbeard doctor -for plan`, then `neckbeard analyze`.
For exact fields use `neckbeard schema app-profile` and `neckbeard schema neckbeard`.
For the actual tier sizing and usage assumptions use `neckbeard presets`.
The skill installer also places these files in this references directory.

## Workload boundary

All three clouds, both VCS providers, and both runtimes are supported by the
planner. This means renderable architecture, not verified deployment. Start with
existing accounts/projects/subscriptions (`entry_path: adopt`). Landing-zone
creation and environment-manifest consumption are not implemented.

An application has one image, up to 10 uniquely named HTTP/worker/cron services,
and repository-root build context. All services declare the same Dockerfile.
Dot-directories (`.devcontainer`, `.github`, …) are never scanned. When a
repository-root `Dockerfile` exists, it is taken as the application image and
every other candidate (`Dockerfile.base`, `packaging/`, a possible second
service image) is listed in a plan-gating `container-images` assumption —
confirm none is separately deployed, or add the real one back as a service and
receive the honest multi-image refusal. Without a root Dockerfile all
candidates stay visible and multi-image is flagged before cloud questions.
HTTP services need a port and health_path; scheduled jobs need a five-field cron.
Leave `command` absent to inherit the image's CMD and ENTRYPOINT. A nonempty
`command` replaces CMD arguments, not ENTRYPOINT. Empty command also inherits.

## Resolve a draft before planning

Every `assumptions` entry must have a `confirmed` answer with the **same id**.
Correct the actual services/needs as well: an answer is evidence, not executable
configuration. Never invent a confirmation on the user's behalf.

**Needs can be missing entirely — review datastores as a first-class step.**
Deterministic detection only sees direct environment reads, quoted
`"DATABASE_URL"` literals, and env-template declarations. Framework helpers
hide the rest (Django's `dj_database_url`, Rails credentials, typed config
wrappers): read the app's actual configuration code (settings.py,
config/runtime.exs, …) and add any datastore the draft missed, with file:line
evidence. A web framework with no database in its draft is almost always a
detection gap, not a stateless app.

**Process roles.** Model long-running daemons (Celery workers, an in-app
scheduler like celery beat) as `kind: worker` with an explicit `command`; use
`kind: cron` only for run-to-completion jobs on a five-field schedule. A
scheduler daemon is typically a singleton — say so in its confirmed entry and
review instance counts at plan/override time; duplicate beat processes mean
duplicate scheduled work.

**The draft's `secrets` list is names only** and errs toward inclusion (plain
config URLs land there too). Keeping extra names is harmless — they become
empty secret containers the operator may ignore — but add any connection
variables the app needs that detection missed.

```yaml
assumptions:
  - id: postgres-mode
    statement: Verify the detected database and decide existing versus new.
confirmed:
  - id: postgres-mode
    question: Is this an existing database?
    answer: Existing managed Postgres; use DATABASE_URL and do not provision it.
needs:
  - capability: postgres
    mode: reference
    secret_name: DATABASE_URL
```

`undecided` is a draft mode only. `plan` refuses it. Use `provision` for catalog
capabilities (postgres, object-storage, secrets), or `reference` with a
`secret_name` for an existing service. Confirm its network reachability separately.
Never include secret values in either profile or configuration.

For unsupported findings retain the evidence and choose one disposition:

```yaml
unsupported:
  - capability: redis
    detected: reads REDIS_URL
    explanation: Redis provisioning is outside the catalog.
    disposition: external
    secret_name: REDIS_URL
    resolution: Existing Redis is operated separately; its endpoint is reachable from the app.
```

`external` provisions a named secret container and records an external reference;
it does not provision, validate, migrate, or manage the external service. Alternatively
use `disposition: not-required` and explain why (for example, test-only code).
Unresolved findings prevent planning. Resolutions remain visible in the blueprint.
Dispositions are resolved inline — no matching `confirmed` entry is needed for
an unsupported finding (confirmed entries pair with `assumptions` ids only).

**How provisioned capabilities reach the app.** A provisioned `postgres` need
binds its connection string into the secret named by the need's `secret_name`
(default `DATABASE_URL`) — set it to whatever variable the app actually reads.
Provisioned `object-storage` and `secrets` expose infrastructure outputs
(bucket names, secret container names); mapping those to app-specific variables
like `AWS_STORAGE_BUCKET_NAME` is not automatic today — wire them through the
app's environment explicitly and say so in the topology review. Resolved
assumptions print as `decision:` lines at plan time; `warning:` is reserved for
things that still need operator attention.

## Configuration example

```yaml
version: 1
app: example
org: example
cloud: aws
region: us-east-1
vcs: github
repo: YOUR-OWNER/YOUR-REPO
runtime: serverless-containers
tier: smallteam
entry_path: adopt
finops:
  currency: USD
```

Replace the example identifiers with the user's choices. For GCP or Azure also
set `containers: {dev: PROJECT-OR-SUBSCRIPTION, stg: ..., prd: ...}` and choose a
region belonging to that cloud. Use short names: cloud resource name limits differ.
Per-cloud region check: on Azure credit subscriptions, verify the region offers
PostgreSQL Flexible Server capacity before planning (a real live finding — see
the findings ledger); AWS and GCP regional gaps surface at V1 plan time.

The available tiers are solo, smallteam, established, regulated. They expand into
explicit resource sizes, availability targets, recovery objectives, and usage
assumptions; read `neckbeard presets` instead of guessing their values. Recovery
objectives in presets are not proof of recovery. Ask about traffic, acceptable
downtime/data loss, and operating capacity in plain language.

## Reviewable output

Run `plan`, `doctor -for estimate`, `estimate`, `scaffold`, then `validate`.
Estimation uses Infracost 0.10.x and its pricing login, but no cloud credentials.
Show unpriced items alongside totals. A missing estimator is not a zero-cost result.

By default scaffold includes a content-addressed catalog under `.neckbeard/catalog/`
and references it with portable relative paths. Commit that directory with infra
and the ownership manifest. Generated modules obey the same no-overwrite contract
as other generated files. An explicit `-catalog` index requires an explicit
`-catalog-source` when rendering; this is a development/custom-catalog escape hatch.

A different bundled catalog requires re-planning; old blueprints are not silently
rendered with new module bytes. After upgrading the CLI, review the resulting diff.

Current deployment limitations: only Azure serverless has a recorded manual live
run, and its database connection was not exercised. Serverless release automation,
full promotion/rollback, cloud budget alerts, and automated recovery are subsequent
work. The generated bootstrap runbook is the next operator step, not a guarantee
that the application has been deployed. Known live findings:
https://github.com/bealesh/neckbeard/tree/main/docs/findings
