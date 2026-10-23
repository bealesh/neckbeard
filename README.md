<p align="center">
  <img src="docs/assets/neckbeard.png" alt="neckbeard" width="320">
</p>

# neckbeard

A plugin for your coding agent that turns an app into cloud infrastructure you
can actually review, with the monthly bill attached.

[neckbeard.sh](https://neckbeard.sh)

Coding agents are great at application code and weirdly confident about
infrastructure. Ask one to "deploy this to AWS" and you'll get three hundred
lines of plausible HCL, a security group open to the world, an IAM policy with
a star in it, and no idea what any of it costs. It reads like the work of
someone who has absorbed every blog post and never once been paged. The model
isn't lying to you. It has simply never seen the invoice.

neckbeard splits the job. Humans maintain a catalog of OpenTofu modules that
are versioned, tested in CI on every change, and scoped to one honest workload
shape: a container image, up to ten HTTP, worker, and cron processes, Postgres,
object storage, secrets. The agent reads your repo, works out what the app
needs, asks you the questions code can't answer, and maps the answers onto
catalog inputs. Everything in between is deterministic Go. The planner writes a
hash-verified `blueprint.yaml`, the renderer is a pure function of it, and
regeneration refuses to overwrite a file you've touched. The agent never
invents infrastructure. If your app needs Redis, you get a named finding that
says the catalog doesn't do Redis yet, rather than a module that looks like it
might.

It covers AWS, GCP, and Azure, with GitHub Actions or GitLab CI, as serverless
containers (Cloud Run, ECS Fargate, Container Apps) or Kubernetes (EKS, GKE,
AKS). The plugin shape is borrowed from
[ponytail](https://github.com/DietrichGebert/ponytail), which proved a plugin
can be mostly one good prompt.

## Install

Claude Code, as two separate prompts:

```
/plugin marketplace add bealesh/neckbeard
```
```
/plugin install neckbeard@neckbeard
```

Then open your app's repo and run `/neckbeard:analyze`. Your first neckbeard
command installs the small CLI the plugin drives (a sha256-verified release
binary, or `go install` if you have Go around), and that's the whole setup.
Cost estimates and validation lean on tools you may already have, namely
Infracost, OpenTofu, and Checkov. `/neckbeard:doctor` tells you what's missing
and what each one buys you.

Codex, Cursor, or anything else that reads a `SKILL.md`: see
[INSTALL.md](INSTALL.md).

## What it's like to use

You run `/neckbeard:analyze` and the agent reads the repo the way a careful
new hire would. Dockerfiles, exposed ports, environment variables, health
endpoints, the Procfile, the corner of `settings.py` where Django quietly
reads `DATABASE_URL`. It writes an app profile in which every fact carries a
file and line number, every inference carries a confidence, and every
assumption is loud. Then it asks you the things it genuinely can't know: how
much traffic to expect, how much downtime you'll tolerate, whether that
Postgres is one you already run or one you want built, what the worker is for.
Your answers get recorded next to the assumptions they resolve, and planning
refuses to run while any of them is still open.

`/neckbeard:estimate` plans against the catalog and shows you the bill before
anything exists. `/neckbeard:sync` scaffolds the infrastructure, the
pipelines, and on Kubernetes the cluster manifests, then validates them and
tells you precisely what it did and didn't prove. From there you review the
files like any other pull request and follow the generated `docs/bootstrap.md`
once per environment, with real credentials in your own hands. The agent never
gets those.

| Command | What it does |
|---------|--------------|
| `/neckbeard:analyze` | Read the repo into an evidence-backed `app-profile.yaml`, ask the open questions, propose `neckbeard.yaml`. |
| `/neckbeard:estimate [comparison]` | Plan against the catalog and show the monthly cost table, including everything that couldn't be priced. `serverless vs kubernetes` or `solo vs smallteam` runs both. |
| `/neckbeard:sync` | Plan, estimate, scaffold, validate. Your edits are never silently overwritten. Ends with PASSED, FAILED, or NOT EXERCISED for each check. |
| `/neckbeard:doctor [plan\|estimate\|validate\|all]` | Install the CLI if it's missing and report which tools are present. |
| `/neckbeard:help` | The short version of this page. |

## Disco on a VPS (experimental)

For a self-hosted [Disco](https://disco.cloud) server, run
`/neckbeard:disco myapp`. It uses the same reviewed app profile and generates
`disco.json` plus `docs/disco.md`, the human-run setup and deployment guide.
Without an agent:

```sh
neckbeard disco -project myapp
```

One image, one HTTP service, optional workers and cron jobs, and Postgres on
the server (or connections to existing services). You supply and operate the
server, including backups. This path has no cost estimate, spending ceiling,
or cloud validation integration. Existing outputs are refused unless you use
`-force`, which replaces both files, including edits. See the
[Disco reference](skills/neckbeard/references/disco.md) for scope and test evidence.

## The bill

neckbeard prices the infrastructure before scaffolding, with editable usage
assumptions and a breakdown of idle, expected, and higher-usage scenarios.

The GCP Umami trial now produces the same estimate from its effective Terraform
and its saved authenticated plan:

| One development environment, USD/month | Idle baseline | Low | Expected | High |
|---|---|---|---|---|
| Umami on Cloud Run + private Postgres + HTTPS | $28.79 | $58.02 | $87.24 | $93.13 |

Expected assumes **730 active instance-hours**, 20 GiB internet transfer,
2 GiB inbound load-balancer traffic, 5 GiB logs, 10 GiB registry storage,
10 GiB backups, and the configured secrets. The single-instance maximum bounds
compute in the high scenario. Free allowances, discounts and taxes are excluded;
this is a conservative usage scenario, not a measured bill.

The [cost model reference](skills/neckbeard/references/cost-model.md) explains the calculations,
regression tests, and remaining validation scope. The
[postmortem](docs/findings/2026-10-22-cost-estimator-postmortem.md) records the
original defects and withdrawn figures. Historical cross-cloud totals have been
removed until the same numerical reconciliation is complete for each lane.

## What it will and won't claim

Infrastructure tooling has a long tradition of implying more than it has
proven. A passing `tofu validate` has never once meant a thing deploys.
neckbeard reports in three levels and insists on showing you the third:

```
V0   PASSED         tofu validate [prd]
V0   PASSED         checkov policy [prd]        every skip carries a written reason
V0   PASSED         kubeconform (clusters/)
V1   NOT EXERCISED  authenticated plan          requires cloud credentials
V2   NOT EXERCISED  deployment verification     requires cloud credentials
```

V0 is syntax and policy. Every Checkov skip in the generated config carries a
written reason, because a skip without one is a lie about your security
posture. V1 is an authenticated plan and V2 is a verified deployment; both
need credentials, and both stay NOT EXERCISED until a human runs them. When
live runs break, the failures go into the public
[findings ledger](docs/findings/) and from there into the catalog. The first
real deployment produced ten of them, and five became module fixes the same
afternoon. Degraded paths say why: an Azure plus GitLab merge-request plan
can't federate, so the generated pipeline says that out loud instead of
failing in a way you'd lose an evening to.

## What's in the box

Three isolated environments with their own state, sized by tier presets that
spell out traffic, availability, recovery objectives, and how many people are
actually around to operate the thing. Every preset value is visible and
overridable within the catalog's tested envelope, and anything outside that
envelope is marked rather than blessed. Pipelines come from one internal model
for either CI: test, build, scan (Trivy blocks HIGH and CRITICAL), push, plans
on pull requests under a read-only identity, applies on main under a write
identity, production gated behind environment protection. There are no static
cloud keys anywhere. It's OIDC federation end to end, and a test fails the
build if a rendered pipeline so much as mentions `AWS_SECRET_ACCESS_KEY`.
Networks are private by default, secret managers hold the names the operator
fills in (values never pass through neckbeard, its state, or your git
history), and managed WAF rules switch on where the tier calls for them. Money
gets two controls that are never confused with each other: `monthly_budget_alert`
renders a real cloud budget per environment that notifies people, and
`monthly_ceiling` is the one that actually refuses, enforced at the only point
a limit can be enforced, before the resources exist.

## Where it actually stands

Pre-alpha, and specific about which parts.

| Claim | Evidence |
|---|---|
| analyze, plan, estimate, scaffold, validate on all 12 cloud × VCS × runtime paths | Static matrix runs in CI on every PR; three unrelated fixture apps onboard end to end |
| Live deploy, update, and rollback on serverless containers | Passed on AWS/GitHub and Azure/GitHub with real database checks ([evidence](docs/tranche-2.md)) |
| Live deploy on GCP, the GitLab lanes, Kubernetes, promotion, recovery drills | Not yet. Implemented, NOT EXERCISED |

The plugin's product today is the front half: reviewable infrastructure,
pipelines, a cost estimate, and a runbook you can hand to whoever holds the
credentials. The deployment lifecycle underneath it (`neckbeard release`,
documented in the skill) exists and has worked for real on two lanes, but it
stays experimental until it clears the gates in the
[launch plan](docs/launch-plan.md). Static validation is not deployment proof.
Landing zones, meaning org structure and guardrails, the Path F story in
[DESIGN.md](DESIGN.md), are designed and not built.

## How it's put together

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
                                   unpriced       never silently  V2 deploy,
                                   named          overwrite       each reported
```

The repository root is the plugin. [`skills/neckbeard/SKILL.md`](skills/neckbeard/SKILL.md)
is the agent's operating manual, [`commands/`](commands/) holds the slash
commands, and the Go CLI under [`cmd/`](cmd/neckbeard) plus the module catalog
under [`catalog/`](catalog/) is the deterministic half. The CLI runs fine
without an agent at all:

```sh
go install github.com/bealesh/neckbeard/cmd/neckbeard@latest
neckbeard analyze && $EDITOR app-profile.yaml neckbeard.yaml
neckbeard plan && neckbeard estimate && neckbeard scaffold && neckbeard validate
```

If you've run anything in production for long enough, you know the expensive
mistakes rarely live in the application code. They live in the account bill
and the 3am page. This is a tool for catching them while they're still a diff.

More, if you want it: [DESIGN.md](DESIGN.md) is the full design, kept honest
since day one. [docs/findings/](docs/findings/) is what reality taught us.
[CONTRIBUTING.md](CONTRIBUTING.md) is how to help.

## License

Apache-2.0, see [LICENSE](LICENSE).
