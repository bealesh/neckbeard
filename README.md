<p align="center">
  <img src="docs/assets/neckbeard.png" alt="neckbeard" width="320">
</p>

<h1 align="center">neckbeard</h1>

<p align="center">
  <em>The neckbeard handles your infra so you don't have to.</em>
</p>

**A plugin for your coding agent that turns an app into reviewable cloud
infrastructure and a monthly cost estimate.** AWS, GCP, or Azure; GitHub Actions
or GitLab CI; serverless containers or Kubernetes — rendered from a bundled
catalog of tested OpenTofu modules. The agent reads your app and explains; the
planning, rendering, and validation are deterministic code. **The agent never
invents infrastructure.**

Inspired by [ponytail](https://github.com/DietrichGebert/ponytail).

## Install

**Claude Code**, as two separate prompts:

```
/plugin marketplace add bealesh/neckbeard
```
```
/plugin install neckbeard@neckbeard
```

Then, in your app's repo: `/neckbeard:analyze`. Your first `/neckbeard:` command installs the
small `neckbeard` CLI the plugin drives (a sha256-verified release binary, or
`go install` when Go is present) — nothing else to set up. Cost estimates and
static validation use tools you may already have (Infracost, OpenTofu, Checkov);
`/neckbeard:doctor` tells you which are missing and what each unlocks.

**Codex, Cursor, or any [SKILL.md](https://developers.openai.com/codex/skills)-speaking agent:** see [INSTALL.md](INSTALL.md).

## What happens

```
you:    /neckbeard:analyze
agent:  reads your repo → evidence-backed profile → asks what code can't tell it
you:    answer the questions, pick a cloud and a tier
you:    /neckbeard:estimate
agent:  plans against the catalog → "$77/mo expected — here's the table, and
        here's what Infracost can't price"
you:    /neckbeard:sync
agent:  scaffold → validate → "V0 PASSED; V1/V2 NOT EXERCISED; next step is
        docs/bootstrap.md"
you:    review the files like any PR; follow the runbook per environment
```

## Commands

| Command | What it does |
|---------|--------------|
| `/neckbeard:analyze` | Reads the repo into an evidence-backed `app-profile.yaml` (facts with file:line, inferences with confidence, loud assumptions), asks the open questions, proposes `neckbeard.yaml`. |
| `/neckbeard:estimate [comparison]` | Plans against the module catalog and shows the monthly cost table — low/expected/high per environment, usage assumptions, and every unpriced resource — before anything exists. `serverless vs kubernetes` or `solo vs smallteam` runs both. |
| `/neckbeard:sync` | The loop: plan → estimate → scaffold → validate, under the ownership contract (your edits are never silently overwritten). Ends with PASSED / FAILED / NOT EXERCISED. |
| `/neckbeard:doctor [plan\|estimate\|validate\|all]` | Installs the CLI if missing; reports which prerequisites are present and what each unlocks. |
| `/neckbeard:help` | The quick reference. |

## Why this exists

Coding agents are good at app code and bad at freehanding infrastructure. The
failure mode is always the same: plausible-looking HCL that is subtly broken,
insecure by default, and unestimable. neckbeard inverts the roles:

- **Humans maintain the golden path** — versioned OpenTofu modules, pipeline
  templates, GitOps layouts, per cloud, tested in CI on every change.
- **The agent analyzes, parameterizes, and explains.** It reads your app,
  figures out what it needs, and maps that onto catalog inputs — with evidence
  for every claim it makes.
- **Everything in between is deterministic.** Same inputs, same bytes: the
  planner emits a hash-verified `blueprint.yaml` lockfile, the renderer is a
  pure function of it, and regeneration never silently overwrites your edits.

Needs the catalog can't express become named findings ("redis is outside the
workload contract"), not force-fits onto the nearest module.

## The part nobody else tells you: what it costs

The same reference app (HTTP service + worker + cron + Postgres), smallest
tier, all three environments, live prices via Infracost:

| | Cloud Run | Container Apps | ECS Fargate | EKS | GKE (regional) | AKS |
|---|---|---|---|---|---|---|
| monthly | **$77** | ~$98\* | $250 | $396 | $754 | ~$801\* |

\* Infracost can't price Container Apps compute yet — and instead of letting a
low number lie, the report **names every unpriced resource**. That's the house
style: baseline-while-idle costs, explicit usage assumptions you can edit,
low/expected/high scenarios, and a provenance footer. It also prices your
*decisions*: this table is why neckbeard tells solo developers "you don't need
Kubernetes yet" — with receipts.

## Honesty as architecture

Infrastructure tools love implying more than they've proven. neckbeard reports
in three verdicts, and the third one is the point:

```
V0   PASSED         tofu validate [prd]
V0   PASSED         checkov policy [prd]        ← every skip carries a written reason
V0   PASSED         kubeconform (clusters/)
V1   NOT EXERCISED  authenticated plan          requires cloud credentials
V2   NOT EXERCISED  deployment verification     requires cloud credentials
```

- Static validation **never claims deployability**. V0/V1/V2 are distinct
  levels, each reported as exercised or not.
- Every Checkov skip in the generated `.checkov.yaml` states its reason. A
  skip without a reason is a lie about your security posture.
- When live runs break, the failures become public
  [findings](docs/findings/) and then catalog fixes. Our first real deployment
  produced ten; five became fixes the same day.
- Degraded paths say why (Azure + GitLab merge-request plans can't federate —
  the pipeline says so instead of failing mysteriously).

## What you get

- **Three isolated environments** (dev/stg/prd) with independent state, sized
  by tier presets over explicit dimensions: traffic, availability, RPO/RTO,
  operating capacity. Every value is visible and overridable within the
  catalog's tested envelope; values outside it are *marked*, not blessed.
- **Two runtimes per cloud**: serverless containers (Cloud Run / ECS Fargate /
  Container Apps) or Kubernetes (EKS / GKE / AKS) with Flux manifests.
- **CI/CD for GitHub Actions or GitLab CI** from one model: test → build →
  scan (trivy blocks HIGH/CRITICAL) → push; infra plans on PRs with a
  read-only identity, applies on main with a write identity, production gated
  by environment protection. **Zero static cloud keys** — OIDC federation
  everywhere, enforced by a test that fails if a rendered pipeline ever
  mentions `AWS_SECRET_ACCESS_KEY`.
- **Security posture as explicit controls**: private-by-default networks,
  secret managers holding *names* the operator fills (values never touch
  neckbeard, its state, or your git history), scoped CI identities, managed
  WAF rules where the tier enables them.
- **A bootstrap runbook** (`docs/bootstrap.md`) that walks a human with
  elevated credentials through state backend → OIDC trust → CI identities, per
  environment. Everything after `validate` is deliberately human-in-the-loop.

The supported workload is **one image, up to 10 HTTP/worker/cron processes,
Postgres, object storage, secrets**, deployed into existing accounts, projects,
or subscriptions. Anything else is an explicit `unsupported` finding.

## Status: pre-alpha, and precise about it

What is proven, and how:

| Claim | Evidence |
|---|---|
| analyze → plan → estimate → scaffold → validate on all 12 cloud × VCS × runtime paths | Static matrix runs in CI on every PR; three unrelated fixture apps onboard end to end |
| Live deploy → update → rollback, serverless | Passed on AWS/GitHub and Azure/GitHub with real database checks ([evidence](docs/tranche-2.md)) |
| Live deploy on GCP, GitLab lanes, Kubernetes, promotion, recovery drills | **Not yet.** Implemented, NOT EXERCISED |

The plugin's product today is the onboarding half: reviewable infrastructure,
pipelines, a cost estimate, and a bootstrap runbook you can hand to a human.
The deployment lifecycle (`neckbeard release …`, documented in the skill) is
experimental and tracked gate by gate in the [launch plan](docs/launch-plan.md).
Do not treat static validation as deployment proof. Landing zones (org
structure, guardrails — Path F in [DESIGN.md](DESIGN.md)) are designed, not built.

## How it hangs together

```
            you / your agent                      deterministic core
        ┌──────────────────────┐            ┌───────────────────────────┐
repo ──▶│ analyze: facts w/    │  profile   │ plan: catalog @ pinned    │
        │ evidence, questions, ├───────────▶│ versions → blueprint.yaml │
        │ judgment, answers    │  config    │ (hash-verified lockfile)  │
        └──────────────────────┘            └─────────┬─────────────────┘
                                       ┌──────────────┼──────────────┐
                                       ▼              ▼              ▼
                                   estimate       scaffold        validate
                                   Infracost,     ownership       V0 static,
                                   scenarios,     contract:       V1 plan,
                                   unpriced       never silently  V2 deploy —
                                   named          overwrite       each reported
```

The repository root is the plugin: [`skills/neckbeard/SKILL.md`](skills/neckbeard/SKILL.md)
is the agent's operating manual, [`commands/`](commands/) are the slash
commands, and the Go CLI under [`cmd/`](cmd/neckbeard) with the module catalog
under [`catalog/`](catalog/) is the deterministic half. The CLI also drives
everything without an agent:

```sh
go install github.com/bealesh/neckbeard/cmd/neckbeard@latest
neckbeard analyze && $EDITOR app-profile.yaml neckbeard.yaml
neckbeard plan && neckbeard estimate && neckbeard scaffold && neckbeard validate
```

Deep dives: [DESIGN.md](DESIGN.md) (the full design, kept honest since day
one) · [docs/findings/](docs/findings/) (what reality taught us) ·
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache-2.0 — see [LICENSE](LICENSE).
