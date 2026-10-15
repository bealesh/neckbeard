# Roadmap

*October 2026. This is an order, not a schedule. Nothing here has a date on
it, because I've shipped enough roadmaps to know what the dates are worth.*

neckbeard went up on Hacker News this week and came back with a couple hundred
stars, a visit from the ponytail author, and a healthy number of people asking
some version of "why would I want this." That last group is the useful one. A
roadmap built from the people who already agree with you is a wishlist. This
one is built mostly from the skeptics.

## What the thread actually said

Grouped, roughly in order of how often it came up and how hard it landed.

**"A Hetzner box is $25 a month and you've got room to spare."** Three
separate people said it, and for a whole class of apps they're right. The cost
table in the README is an argument for staying small, and the smallest honest
number for a side project isn't on any of the three clouds we support. One of
them offered their hand-built Hetzner setup as reference material. Taking them
up on it.

**"Great way to wake up and find your cloud spend has skyrocketed because an
LLM assumed wrong."** The architecture already stops the model from inventing
resources, and the estimate runs before anything exists. But the budget alert
story in the docs still says "planned," and planned isn't a control. The
schema even has a `monthly_budget_alert` field the CLI quietly ignores. That's
the kind of thing this project is supposed to be allergic to.

**"Why would I want another layer between me and my HCL?"** From someone
running a couple hundred AWS accounts over GitLab with foundation, team, and
application layers managed by different people. The honest answer is that
neckbeard isn't for that shop yet. It's for the app that started on Vercel and
Supabase and just got real. But the three-layer model they described is more
or less Path F in [DESIGN.md](../DESIGN.md), and it's the most concrete
description of a landing zone anyone has handed me. It goes in the later pile
with a clearer shape than it had last week.

**"Can I use it with ponytail?"** Yes, and that's the intended pairing:
ponytail for the application code, neckbeard for what it runs on. Except
nobody has loaded both plugins into one session and checked that they get
along, which is exactly the kind of claim this project isn't supposed to make
unverified.

**"Why a plugin? Just ask the agent, it works fine."** And its cousin,
"plugins are just prompts and shell scripts." Fair on the mechanics. The pitch
is the catalog and the determinism, and right now you have to read the repo to
find that out. ponytail answered the same question with a benchmark. We should
too, and we should publish the runs where the plugin doesn't help.

**"The README is cringe."** It was. Fixed in #23.

**"The beard is the wrong color."** Noted. The beard stays.

## Now

**1. A Hetzner lane.** A fourth `cloud` with one runtime: containers on a VPS
via the hcloud provider and cloud-init, a private network, a firewall, and
Postgres running on the box with scheduled backups to Hetzner Object Storage,
because there's no managed Postgres to lean on. That's a different durability
story from RDS and the tier presets will say so instead of pretending. Infracost
doesn't price Hetzner, so the estimate will come from a maintained price table
with a date on it, labeled as list price. Same V0 gates as the other lanes.
This is the item I'd most like a pull request for. Open an issue first so we
agree on the shape before anyone writes HCL.

**2. Budgets that exist.** Implement `finops.monthly_budget_alert` for real:
AWS Budgets, GCP billing budgets, Azure cost alerts, rendered per environment.
Add `finops.monthly_ceiling`, which makes `scaffold` refuse when the expected
scenario exceeds it. Alerts notify and don't cap, per DESIGN §10.2, and the
report will keep saying that in plain words. The ceiling is the cap, and it
lives at the only point where a cap can actually be enforced: before the
resources exist.

**3. Prove the ponytail pairing.** Install both plugins into a clean config,
onboard a fixture app, and check that ponytail's "smallest change that works"
doesn't talk the agent out of neckbeard's "never hand-write infrastructure."
Write down what happened in INSTALL.md. If they fight, fix the skill, not the
docs.

**4. Land the outside contributions.** #17 (Procfile roles) and #18
(docker-compose evidence) are reviewed and waiting on a rebase. #20 collapses
the analyzer's growing argument list into a struct so the next detection PR
doesn't have to touch nine parameters.

**5. Dogfood the plugin socially, not just mechanically.** The plugin passes
strict validation and installs end to end, and the skill has been followed by
a fresh agent on one app I didn't write. That's one. It needs to be run on a
few more repos by people who aren't me, with the friction written down.

## Next

**6. A benchmark.** Same fixture apps, same agent, same prompts, with and
without the plugin. Measure tokens, wall time, whether the output passes V0,
whether there's a star in an IAM policy, and what the result costs per month.
Publish the method and the per-task tables, including the ones we lose.

**7. The live matrix.** The [launch plan](launch-plan.md) gates don't move:
GCP/GitHub is mid-flight, then the GitLab lanes, then Kubernetes. Live
acceptance is 0 of 12 today and the README will say so until a harness run
says otherwise.

**8. DigitalOcean.** The second small-cloud lane, if the Hetzner shape holds
up. Managed Postgres exists there, which makes it closer to the big three than
Hetzner is.

## Later, maybe

**9. Landing zones.** Path F, with the three layers from the thread: a
foundation layer for networking, IAM boundaries, and logging; a team layer for
budgets, people-to-role mapping, and firewall exceptions; and the application
layer, which is what neckbeard does today. Requires org-root authority and a
platform repo, and the docs will be appropriately scary about it. Doesn't
start until the application layer is 12 of 12.

**10. Self-hosted PaaS as a target.** Our output is already "a Docker image
plus everything around it." Something like Disco might be a renderer rather
than a cloud. Playing with it first; no promises.

## Not doing

Virtual machines configured with Ansible or baked with Packer. If that's your
shop and it works, keep it. Replacing OpenTofu you already maintain and like.
Enterprise networking, meaning transit gateways, DMZ layering, and directory
forests, ahead of landing zones. Redis, queues, MySQL, Mongo, and GPUs stay
named findings until a module earns a place in the catalog by being tested
like the others.

## How to help

Pick something from Now, open an issue describing the shape, and we'll agree
on it before you write the code. Catalog modules get the same treatment as
everything else here: fmt, validate, Checkov with reasoned skips, and a lane in
the CI matrix. The bar is "I'd let this run against my own account," which is
also the bar for everything already in here.

If you read this far and your reaction was "that's a lot of yak for a side
project," you're right, and that's sort of the point. The expensive part of
infrastructure was never the HCL.
