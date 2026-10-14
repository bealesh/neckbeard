---
description: Run the neckbeard loop — plan, estimate, scaffold, validate — and walk me through what changed, what it costs, and what is NOT EXERCISED.
argument-hint: "[what changed, or 'skip estimate']"
---

Load the neckbeard skill and run its plan → estimate → scaffold → validate loop
against the current `neckbeard.yaml` and `app-profile.yaml` (if `neckbeard` is
not on PATH, install it first as the skill's Prerequisites say):

- Show me the topology summary and the estimate's scenario table (plus the
  unpriced/unmodeled section) BEFORE scaffolding. If the estimator is
  unavailable, say so and ask before continuing without numbers.
- Apply the ownership contract strictly: resolve scaffold conflicts
  (`<file>.neckbeard-new`) with me rather than deleting my edits; never touch
  `blueprint.yaml` by hand.
- Finish with the validate report, spelling out exactly what PASSED, FAILED,
  and was NOT EXERCISED — V0 proves syntax and policy, never deployability —
  and what the next human step is (usually the generated `docs/bootstrap.md`).

$ARGUMENTS
