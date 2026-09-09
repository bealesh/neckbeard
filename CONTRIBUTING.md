# Contributing to neckbeard

Thanks for looking under the hood. Ground rules, kept short:

## Workflow

- **Everything lands via pull request** — including from maintainers. No direct
  pushes to `main`.
- Branch names: `feat/…`, `fix/…`, `chore/…`, `docs/…`.
- CI must be green: `go test ./...`, `gofmt`, `go vet`, `tofu fmt -check` over
  the catalog, and the six-lane plan → scaffold → V0 validate matrix.
- Squash-merge; the PR title becomes the commit subject.

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
