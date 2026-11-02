# Benchmarks

"Why a plugin? Just ask the agent, it works fine." That was the most common
pushback when neckbeard went public, and it is fair on the mechanics: a capable
agent will write you Terraform if you ask. The pitch was never that the agent
can't. It is that what comes back is different, and you should be able to see
how without reading the catalog. So this directory runs the same agent on the
same apps with the same prompts, with and without the plugin, and measures
what comes back.

Results live in [results/](results/), one dated directory per run of the
matrix: a write-up, the generated tables, and under `raw/` every run's metrics
and the agent's own closing summary. The first is
[2026-11-02](results/2026-11-02-onboarding/README.md). Read the limits section
of a result before the tables. These are small numbers from one machine and
one model, not a study.

## What is measured

Each run drops a fresh copy of a fixture app into an empty directory, starts
Claude Code non-interactively with a task prompt, and lets it finish. The
plugin arm loads this repository as a Claude Code plugin and has the
`neckbeard` CLI on `PATH`. The bare arm has neither. Everything else is held
equal: the model, the prompt, the tools, a scrubbed environment with no cloud
credentials, and the cloud CLIs denied outright so that even a determined run
cannot touch an account. Nothing deploys.

When the agent is done, [score.py](score.py) applies the same checks to both
arms' output. From Claude Code's own result record it takes cost, tokens,
turns and wall time. On the files it finds OpenTofu roots and runs `tofu init`
and `tofu validate` on each, `tofu fmt -check`, checkov with the terraform
framework, and actionlint over any workflows. It scans HCL and JSON for IAM
wildcards, counted three ways because they are not equally bad: `Action: *`,
`service:*`, and `Resource: *`. It scans for plaintext secrets with a handful
of patterns and a placeholder filter, so `password = "changeme"` does not
count and `postgres://app:hunter2@` does. It counts the lines of HCL and
workflow YAML a reviewer would have to read. When Infracost can price the
output it does, per root, and reports the dev environment. For the analysis
task it checks the agent's answer against a hand-written list of facts about
the app, in [facts/](facts/), and counts citations that carry a line number.

"Passes V0" means what it means in neckbeard: every root validates, checkov
reports zero failed checks, and actionlint is clean. The plugin's generated
`.checkov.yaml` skips are honoured when present, with their written reasons,
because that file is part of what the plugin produces. The bare arm gets the
same courtesy if it writes one.

## The tasks

Three prompts, in [prompts/](prompts/), chosen so the plugin should help on
some and not others.

**onboard** is the whole job: production on AWS for a solo developer, three
environments, CI that builds and deploys without stored keys, and a monthly
estimate, as reviewable code. This is what neckbeard exists for. It runs on
three apps: bellwether, the repository's own reference app (Go, HTTP plus a
worker plus a nightly job, Postgres); umami, a real analytics product nobody
here wrote (Next.js, Prisma, Postgres, migrations at startup), pinned to one
commit; and a one-file Python service, because tiny apps are where
over-engineering shows.

**needs** asks what the app needs to run in production, with file and line
evidence, and nothing else. The plugin's analyzer gives a deterministic draft
here, but a good agent reads code fine on its own, so the expected gap is
small.

**static-site** asks for S3 behind CloudFront with a certificate and DNS for
a folder of HTML. Neckbeard does not do this. The plugin should say so by name
rather than bend its catalog around it, and the bare agent should just do it.
This is the task where the plugin is supposed to lose, and the table says
whether it lost honestly.

## Running it

```sh
benchmarks/run.sh prepare          # copies the fixtures, clones umami at the pinned commit, builds the CLI
benchmarks/run.sh matrix 3 4       # every cell, three runs each, both arms, four agents at a time
python3 benchmarks/aggregate.py benchmarks/.work/runs benchmarks/results/<date>/raw   # tables + raw copies
```

`CLAUDE_BIN` points at a Claude Code binary if `claude` is not on `PATH`.
`MODEL` pins a model for both arms; otherwise both use your configured
default, and the result record says which. A full matrix is 36 agent runs,
about six agent-hours and around 150 USD on your account at Fable prices.
Outputs stay under `benchmarks/.work/`, which is ignored; the committed
results carry the metrics and each run's closing summary, not the generated
trees. `benchmarks/run.sh score <run-dir>` re-scores a run without touching
the agent, which is how scorer fixes get applied to finished runs.

The matrix is a job queue, not a loop. Every run is a job, the bare and plugin
arms are interleaved so the long bare runs do not gate a cell, and scoring
happens afterwards in its own parallel pass so no agent waits on checkov. One
OpenTofu provider cache serves every `tofu init` in every run. Four agents at
once was comfortable on one account; six produced rate-limit events. If you
are changing the harness, smoke it on a cheaper model with one run per cell
first; three of the scorer's bugs on the first matrix were found on live
Fable runs that a Sonnet pass would have caught for a tenth of the cost.

## What this does not show

Infracost needs a working login and the column is empty when it has none.
The scorer prefers the 0.10 line of the CLI, the same one neckbeard's
estimator uses, because the 2.x CLI routes every breakdown through a dashboard
login. Re-scoring with `run.sh score` fills the column in once that works; the
agent runs do not need repeating. The number is a raw breakdown of what each
arm provisioned in dev with no usage file, which is a different and smaller
thing than either arm's own estimate.

It does not show whether anything deploys. V0 is syntax, schema and policy,
and the README says so everywhere else too. It does not grade the quality of
an architecture beyond what checkov and a wildcard scan can see; an agent can
pass every check here with a design you would not run. The fact lists for the
analysis task are short and written by the same person who wrote the plugin.
And the sample is small: a handful of runs per cell on one model on one day.
Treat differences inside the min–max range as noise.
