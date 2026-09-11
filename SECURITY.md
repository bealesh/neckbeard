# Security policy

## Reporting a vulnerability

Use GitHub's private vulnerability reporting: **Security → Report a
vulnerability** on this repository. Please do not open a public issue for
anything you believe is exploitable.

This is a pre-1.0 project maintained on a best-effort basis. You'll get an
acknowledgment as soon as a maintainer reads the report — there is no staffed
SLA yet, and this file will say so honestly until there is.

## Scope

Reports are especially welcome on:

- **Generated infrastructure posture** — a catalog module that provisions
  something less secure than its documentation claims, or a checkov skip whose
  written reason doesn't hold.
- **CI trust boundaries** — the OIDC federation shapes in `catalog/*/bootstrap`,
  the generated pipelines in `core/pipeline`, and this repository's own
  workflows. The invariant that pull requests run credential-free (see
  CONTRIBUTING.md) is enforced by the `fork-safety` CI job; a way around it is
  a vulnerability.
- **The release runner** (`core/release`) — any path where a secret value could
  reach argv, a file that outlives the operation, version-controlled files, or
  OpenTofu state.

## Supported versions

Pre-release: only `main` receives fixes. Versioned support windows will be
documented when tagged releases exist.
