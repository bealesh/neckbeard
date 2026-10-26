---
description: Plan against the catalog and show what this app would cost per month — scenario table, unpriced resources, and what changes it — before anything is scaffolded.
argument-hint: "[compare: e.g. 'serverless vs kubernetes' or 'solo vs smallteam']"
---

Load the neckbeard skill. Using the current `neckbeard.yaml` and
`app-profile.yaml` (if either is missing, stop and point me at
`/neckbeard:analyze`):

1. `neckbeard plan` — walk me through the per-environment topology summary and
   every `decision:` / `warning:` line.
2. `neckbeard doctor -for estimate`. If Infracost 0.10.x is missing or not
   logged in, say so plainly and give me the install/login commands; a missing
   estimator is NOT a zero-cost result, and you must not guess prices.
3. `neckbeard estimate -root .` — then show me, verbatim from `costs/estimate.md`: the
   scenario table (low / expected / high per environment and total), the usage
   assumptions the tier implies, and the "unpriced / unmodeled" section. Say in
   one sentence which line items dominate and which tier or runtime lever would
   move them. Explain active instance-hours, job execution count and duration rather
   than inferring them from request count. Reports marked PARTIAL are subtotals,
   cannot approve a ceiling, and must never be presented as a full monthly bill.
   For an existing deployment, compare using `neckbeard estimate -plan <plan.json>
   -env <environment>`; keep the authenticated plan local.
4. If I asked for a comparison, run the alternative WITHOUT touching my files:
   copy `neckbeard.yaml` to a scratch path with the one changed field, then
   `neckbeard plan -config <scratch.yaml> -out <scratch-blueprint.yaml>` and
   `neckbeard estimate -config <scratch.yaml> -blueprint <scratch-blueprint.yaml>
   -out <scratch-report.md>`, and present both expected totals side by side
   with the reason for the difference.

Do not scaffold. End by telling me the next step is `/neckbeard:sync` once I
am happy with the numbers.

$ARGUMENTS

Cost model v3 covers AWS/Azure/GCP serverless and Kubernetes. Before presenting
an estimate, review `active_instance_hours` (per HTTP service), job executions
and duration, and `additional_node_hours` (aggregate hours above minimum nodes).
Workers and minimum nodes remain billable at idle. Check log queries, object
operations, custom metric series and explicit unused-feature assumptions in the
report. Unused inputs for another runtime add no charge; missing billable
components make coverage partial. See the
[cost model](../skills/neckbeard/references/cost-model.md) and distinguish pricing evidence from deployed billing.
