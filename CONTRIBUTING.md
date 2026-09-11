# Contributing to neckbeard

Thanks for looking under the hood. Ground rules, kept short:

## Workflow

- **Everything lands via pull request** — including from maintainers. No direct
  pushes to `main`.
- Branch names: `feat/…`, `fix/…`, `chore/…`, `docs/…`.
- CI must be green: `go test ./...`, `gofmt`, `go vet`, `tofu fmt -check` over
  the catalog, and the six-lane plan → scaffold → V0 validate matrix.
- Squash-merge; the PR title becomes the commit subject.

## CI and cloud safety

The permanent invariant: **pull requests never touch a cloud account.**

- **Tier 0 — every PR and push, fork-safe.** gofmt/vet/tests, catalog
  formatting, and the six-lane plan → scaffold → V0 matrix. These jobs are
  credential-free by construction: this repository holds no cloud secrets, jobs
  run with `contents: read` only, and no cloud OIDC provider anywhere trusts
  this repository — a malicious PR workflow has nothing to steal and nowhere
  to spend.
- **Tier 1 — live deployment tests.** These cost real money and run against
  dedicated private fixture repositories and disposable accounts through the
  release harness, dispatched by a maintainer. They are never triggered by a
  pull request and are never required PR checks; their results land as evidence
  in `docs/` (findings, run IDs, the coverage table).
- The `fork-safety` CI job enforces the invariant mechanically: it fails any
  workflow that uses `pull_request_target`, or that mixes a `pull_request`
  trigger with `id-token` or `secrets.`. If your change legitimately needs
  credentials in CI, it belongs in the harness, not here.
- Workflow actions are pinned by commit SHA and tool installs by version;
  Dependabot proposes bumps and a human merges them. Don't float anything.

## Before you open a PR

```sh
make build test
tofu fmt -check -recursive catalog
```

If you touched the planner or renderers, regenerate goldens deliberately
(`make golden-update`) and include them in the diff — golden churn is reviewed,
never rubber-stamped.

## Project principles (the short version of DESIGN.md)

1. **The agent never invents infrastructure.** Humans maintain the catalog;
   planning/rendering/validation are deterministic code. Same inputs, same bytes.
2. **Honesty over polish.** Anything not exercised is reported NOT EXERCISED.
   Every checkov skip carries a written reason. Placeholder pins say so.
3. **Never silently overwrite.** The ownership contract (DESIGN §6) is load-bearing;
   changes to it need a very good story.
4. **Findings are assets.** If a live run breaks, the failure goes into
   `docs/findings/` and becomes a harness requirement.

## Catalog changes

Module input schemas and resource addresses are contractual within a major
catalog version (§6.3). Anything that moves a resource address is a major bump
with a written migration path — no exceptions, including "it's just a rename."

## License

Apache-2.0. By contributing you agree your contributions are licensed under it.
