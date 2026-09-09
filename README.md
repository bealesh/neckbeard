# neckbeard

A plugin for your favorite coding agent that analyzes your application and scaffolds
cloud infrastructure with explicit, tested controls — environments, CI/CD, GitOps,
observability — with an honest cost estimate attached.

Sibling of ponytail. The neckbeard handles your infra so you don't have to.

**Status: pre-alpha.** M0 (contracts, deterministic planner, ownership/regeneration
engine) complete; M1 (real OpenTofu rendering + the six-lane vertical slice) is next. Read
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
cd $(mktemp -d) && cp $OLDPWD/core/planner/testdata/basic/*.yaml . 
$OLDPWD/bin/neckbeard plan -catalog $OLDPWD/catalog/index.yaml   # → blueprint.yaml
$OLDPWD/bin/neckbeard scaffold                                   # → files + .neckbeard/manifest.json
echo "edit" >> docs/topology.md
$OLDPWD/bin/neckbeard scaffold   # refuses: conflict + docs/topology.md.neckbeard-new
```

`scaffold` follows the regeneration contract (DESIGN §6.2): generated files are
replaced only while unmodified; hand-edited or foreign files are never overwritten —
the fresh render lands beside them as `<file>.neckbeard-new` and the run exits
nonzero until resolved. `custom.tf` extension points are created once and never
touched again. `blueprint.yaml` is hash-verified: edit `neckbeard.yaml`, not the
lockfile.

## License

Apache-2.0 — see [LICENSE](LICENSE). Contributions land via pull request; see
[CONTRIBUTING.md](CONTRIBUTING.md).
