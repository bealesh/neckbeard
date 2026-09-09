# Tranche 1 — usable onboarding

Implemented 2026-09-09. The scope is taking an unfamiliar, supported application
from analysis to reviewable infrastructure without needing a neckbeard source
checkout. Live deployment is the next tranche.

| Deliverable | Result |
|---|---|
| Self-contained distribution | The binary embeds the catalog, schemas, presets, and skill guidance. Skill installation includes the reference files. |
| Portable infrastructure | Scaffold copies catalog contents into `.neckbeard/catalog/<digest>/` and uses relative module sources. The blueprint pins the digest; mismatched CLI/catalog bytes require re-planning. Estimates use the same bundle in a temporary directory. |
| Prerequisite checks | `doctor -for plan`, `estimate`, `validate`, or `all` reports local requirements and incompatible OpenTofu/Infracost versions. `schema` and `presets` expose exact supported inputs. |
| Ordinary applications | Omitted or empty commands inherit image defaults on all runtimes. Bellwether now declares its special commands explicitly. AWS worker-only apps no longer require ingress inputs. |
| Reviewed decisions | Planning blocks unresolved assumptions, undecided datastore modes, and unsupported findings without a disposition. Existing services require an explicit connection secret name. Decisions remain visible in the blueprint and topology. |
| Early workload boundary | Missing Dockerfiles and multiple images are flagged before cloud configuration questions. Draft evidence is retained. Landing zones and manifest consumption are explicitly refused. |

## Verification

- Full Go test suite and `go vet ./...` pass; Go and catalog formatting checks pass.
- Built-binary tests run from unrelated directories containing three non-Bellwether
  fixtures: a Node HTTP app, Python with an existing database, and Node with an
  unsupported Redis dependency. Each exercises all 12 cloud/VCS/runtime
  combinations. Tests cover deterministic planning, review gates, installed
  references, portable scaffolds, image defaults, and estimator scratch contents.
  Pricing is stubbed in these automated tests; database/Redis I/O is not exercised.
- OpenTofu 1.12.6 initialized and validated generated dev and bootstrap roots for
  all six cloud/runtime combinations using cached real providers and disabled
  backends. An additional AWS worker-only dev root passed. These are static checks,
  not authenticated plans or deployment tests.
- Checkov 3.3.10 checked the bundled AWS serverless dev fixture: 73 checks passed,
  zero failures. Optional online guideline retrieval was unavailable; local checks ran.
- Infracost 0.10.45 produced a real four-scenario report across dev/stg/prd for the
  synthetic AWS app using an external database, without an external catalog path.
- The existing six-lane CI job now uses the bundled catalog, and generated infra
  workflows watch bundled module and policy-file changes as well as `infra/`.

## Upgrade and next step

Catalog revision 0.2.0 changes the default command behavior. Re-plan old blueprints
with the new binary, add explicit commands if an app relied on the old
`serve`/`work`/`report` convention, and review the scaffold diff. For existing
connections, set `secret_name` explicitly instead of relying on a generated name.
Commit the bundled catalog alongside generated infrastructure.

Review follow-up: scaffolding currently includes all three clouds' modules under
one catalog digest. Per-cloud bundles could reduce repository and policy-review
noise; introduce them with corresponding digest and upgrade tests when reducing
the generated footprint becomes a priority. The full bundle remains intentional
for tranche 1.

The AWS account was not used and no cloud resources were created. Tranche 2 can
now tackle real bootstrap, image publication/deployment, application and database
verification, update, and teardown. Serverless release automation, promotion and
rollback, cloud budget alerts, and recovery verification remain subsequent work.

User-facing setup: [onboarding guide](../plugin/skills/neckbeard/references/onboarding.md).
