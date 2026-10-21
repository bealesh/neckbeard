# Ponytail pairing — 2026-10-21

## Setup

Claude Code 2.1.286 ran with an initially empty `CLAUDE_CONFIG_DIR`, no user
settings or MCP servers, and both plugins loaded via `--plugin-dir`: neckbeard
at Procfile candidate `a47a6f2e20130d05ba76d51c870d6fe21db1c9b6` and ponytail
5.3.2. The existing Claude login authenticated the test through an in-memory
environment variable; credentials were not written into the fixture or evidence.
The fixture was the shipped Python onboarding app in a disposable directory.
Cloud credential environment variables were excluded.

The prompt requested `/neckbeard:analyze` followed by the complete local loop.
It supplied confirmed decisions: one HTTP service on port 8000 at `/healthz`,
existing PostgreSQL via `DATABASE_URL`, AWS/GitHub serverless containers, adopt
path, and the solo tier. It authorized scaffold without a price if the estimator
was unavailable, but a real Infracost estimate succeeded.

## Results

- Both SessionStart hooks and ponytail's UserPromptSubmit hook completed without
  error. Ponytail full mode was present in the session context.
- The agent loaded neckbeard's skill and onboarding reference, ran the CLI's
  analyzer, and refined the evidence-backed profile and configuration.
- Plan and the Infracost 0.10.45 estimate passed. The agent showed the monthly
  scenarios and unmodeled inputs before scaffolding.
- Only `app-profile.yaml` and `neckbeard.yaml` were written through the agent's
  editing tools. The CLI generated the blueprint, infrastructure, workflows,
  catalog copy, release helpers, and documentation.
- V0 passed: OpenTofu formatting, provider initialization and validation for
  dev/stg/prd and their bootstrap stacks; Checkov for all six stacks; actionlint
  with six explicitly reported `queue: max` compatibility exclusions.
- No infrastructure was applied, deployed, or hand-written. V1 authenticated
  planning, V2 deployment, database reachability, and real cloud behavior remained
  NOT EXERCISED, and the final agent report stated those limits.
- No observed conflict between ponytail's minimal-change guidance and
  neckbeard's deterministic generation contract. Ponytail's optional skills
  were not explicitly invoked; its session guidance was active.

## Friction and limits

The test allowed Read/Edit/Write/Glob/Grep/Skill and `Bash(neckbeard *)`, with
delegation tools unavailable. Two attempted shell operations (a heredoc edit
and an extra `echo`) were denied by that harness allowlist. The agent used the
normal file tools and a standalone CLI command instead. These were harness
permission restrictions, not plugin failures.

Validation exceeded the first shell call's 120-second foreground window and
continued in the background. The agent polled its output, then unnecessarily
started a second validation with a longer foreground timeout. Both completed
successfully. This duplicated local static checks, not cloud operations, and
is recorded rather than presented as a frictionless run.

This verifies one fixture, configuration, CLI version, and pair of plugin
versions. Decisions were supplied up front and the test was run by the
maintainer's automation. It does not count as an independent-user trial for
issue #28, nor establish compatibility with every ponytail skill or cloud lane.
