---
description: Analyze this repository into an evidence-backed neckbeard app profile and config — the onboarding step before estimate/scaffold.
argument-hint: "[notes about the app, or a subdirectory]"
---

Load the neckbeard skill and perform the analyze step of its flow on this
repository:

0. If `neckbeard` is not on PATH, install it first exactly as the skill's
   Prerequisites say (the plugin's `scripts/install.sh`); never hand-write a
   profile without the CLI's draft.
1. Run `neckbeard doctor -for plan`, then `neckbeard analyze` for the
   deterministic draft (respect an existing refined `app-profile.yaml` —
   refine in place rather than -force).
2. Read the code behind every draft fact; correct service kinds, ports, health
   paths, commands, and schedules with evidence. Review datastores as a
   first-class step — a web framework with no database in its draft is almost
   always a detection gap. Keep the facts / inferences / assumptions /
   confirmed discipline.
3. Ask me the open questions (traffic, availability, RPO/RTO, workers/cron,
   provision-vs-reference for each datastore) and record my answers under
   `confirmed:` with the matching assumption ids. Resolve unsupported findings
   with an explicit disposition and explanation; preserve their evidence.
4. Propose `neckbeard.yaml` (cloud, region, runtime, tier, repo, containers)
   with your reasoning — especially the runtime and tier recommendations — and
   wait for my go-ahead before running `neckbeard plan`. Next step after that:
   `/neckbeard:estimate` (the money, before anything is generated).

$ARGUMENTS
