# neckbeard

A plugin for your favorite coding agent that analyzes your application and scaffolds
cloud infrastructure with explicit, tested controls — environments, CI/CD, GitOps,
observability — with an honest cost estimate attached.

Sibling of ponytail. The neckbeard handles your infra so you don't have to.

**Status: pre-alpha.** M0 (contracts + deterministic planner) in progress. Read
[DESIGN.md](DESIGN.md) first — the thesis in one line: humans maintain a tested,
versioned module catalog; the agent analyzes, parameterizes, and explains; planning,
rendering, and validation are deterministic code. The agent never invents
infrastructure.

## Layout

- `schemas/` — JSON Schema contracts for every durable artifact (`neckbeard.yaml`,
  `app-profile.yaml`, `blueprint.yaml`, environment manifest)
- `catalog/` — the golden path: pinned module index (real OpenTofu modules land in M1)
- `core/` — deterministic planner, presets, blueprint (Go)
- `cmd/neckbeard` — the CLI the agent plugin shells out to

## Try it

```sh
make build test
bin/neckbeard plan \
  -config core/planner/testdata/basic/neckbeard.yaml \
  -profile core/planner/testdata/basic/app-profile.yaml \
  -catalog catalog/index.yaml -out /tmp/blueprint.yaml
```

