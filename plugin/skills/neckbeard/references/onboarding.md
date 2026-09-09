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
Multiple Dockerfiles are flagged before asking cloud questions: identify which
one builds the application, or explain that separate images are unsupported.
HTTP services need a port and health_path; scheduled jobs need a five-field cron.
Leave `command` absent to inherit the image's CMD and ENTRYPOINT. A nonempty
`command` replaces CMD arguments, not ENTRYPOINT. Empty command also inherits.

## Resolve a draft before planning

Every `assumptions` entry must have a `confirmed` answer with the **same id**.
Correct the actual services/needs as well: an answer is evidence, not executable
configuration. Never invent a confirmation on the user's behalf.

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
