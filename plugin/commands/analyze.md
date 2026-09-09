---
description: Analyze this repository into an evidence-backed neckbeard app profile and config — the onboarding step before plan/estimate/scaffold.
---

Load the neckbeard skill and perform the analyze step of its flow on this
repository:

1. Run `neckbeard analyze` for the deterministic draft (respect an existing
   refined `app-profile.yaml` — refine in place rather than -force).
2. Read the code behind every draft fact; correct service kinds, ports, health
   paths, commands, and schedules with evidence. Keep the
   facts / inferences / assumptions / confirmed discipline.
3. Ask me the open questions (traffic, availability, RPO/RTO, workers/cron,
   provision-vs-reference for each datastore) and record my answers under
   `confirmed:`.
4. Propose `neckbeard.yaml` (cloud, region, runtime, tier, repo, containers)
   with your reasoning — especially the runtime and tier recommendations — and
   wait for my go-ahead before running `neckbeard plan`.

$ARGUMENTS
