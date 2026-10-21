---
description: Quick reference for the neckbeard commands, the flow, and what each step proves.
---

Show the neckbeard quick reference. One shot, change nothing, run nothing:

**The flow** — `/neckbeard:analyze` (read the repo into an evidence-backed
profile; ask what code can't tell) → `/neckbeard:estimate` (plan against the
module catalog and show the monthly cost table before anything exists) →
`/neckbeard:sync` (plan → estimate → scaffold → validate under the ownership
contract) → the generated `docs/bootstrap.md` (a human with elevated
credentials sets up state, OIDC federation, and CI identities per environment)
→ the deployment lifecycle in the skill's release reference.

**Commands** — `/neckbeard:analyze`, `/neckbeard:estimate [comparison]`,
`/neckbeard:sync`, `/neckbeard:doctor [plan|estimate|validate|all]` (installs
the CLI if missing; reports the rest of the toolchain), `/neckbeard:help`.

**Disco (experimental)** — `/neckbeard:disco [project-name]` renders the reviewed
profile as `disco.json` and `docs/disco.md` for a human to deploy on a VPS.
It bypasses cloud planning, estimates, and validation; `-force` replaces both
outputs without scaffold's ownership protection. See the skill's Disco reference.

**What it proves** — V0 (static: fmt, validate, checkov, kubeconform) is what
the plugin runs; it never claims deployability. V1 (authenticated plan) and V2
(deployment verification) need cloud credentials and are reported NOT
EXERCISED until a human runs them. The agent never hand-writes infrastructure:
every module comes from the versioned catalog, the planner emits a
hash-verified `blueprint.yaml`, and regeneration never silently overwrites
edits.

**Supported workload** — one image, up to 10 HTTP/worker/cron processes,
Postgres, object storage, secrets. AWS / GCP / Azure × GitHub / GitLab ×
serverless containers / Kubernetes. Anything else becomes a named
`unsupported` finding with a disposition, not a force-fit.

Point at https://github.com/bealesh/neckbeard for the README, design, and the
live findings ledger.
