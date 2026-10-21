---
description: Render a reviewed app profile for Disco and explain the human-run deployment steps.
argument-hint: "[Disco project name]"
---

Load the neckbeard skill and read its Disco reference (`references/disco.md`).

- Install the CLI as the skill's prerequisites describe. Check that
  `neckbeard disco -h` exists; if missing, explain that the CLI needs an upgrade.
  Never hand-write disco.json as a fallback.
- Analyze and refine `app-profile.yaml` using the evidence and confirmation rules.
  Respect existing files; resolve all open assumptions.
- Confirm the project name and the server choice, cost, and backup responsibility
  with me. Disco has no neckbeard estimate or spending ceiling integration.
- Run `neckbeard disco -project <confirmed-name>` from the repository root.
  Review existing outputs with me before using `-force`, which replaces both
  files. This command does not use scaffold's ownership contract.
- Review `disco.json` and `docs/disco.md` with me. The human runs the server,
  datastore, secret-setting, and deployment commands. Never request secret values
  or execute the generated runbook yourself. Report deployment NOT EXERCISED
  until there is evidence for this app.

$ARGUMENTS
