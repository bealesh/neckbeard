# Install

neckbeard is a skill plus a small CLI. The skill is the agent's operating manual
([`skills/neckbeard/SKILL.md`](skills/neckbeard/SKILL.md)); the CLI does the
deterministic work (plan, render, validate, estimate). Pick your agent.

## Claude Code

```
/plugin marketplace add bealesh/neckbeard
```
```
/plugin install neckbeard@neckbeard
```

Send them as two separate prompts. Then `/neckbeard:analyze` in your app's repo.
The plugin installs the CLI on first use via `scripts/install.sh` (a
sha256-verified release binary into `~/.local/bin`, or `go install` when Go is
present); `/neckbeard:doctor` reports the rest of the toolchain.

## Running alongside ponytail

The intended pairing is [ponytail](https://github.com/DietrichGebert/ponytail)
for application code and neckbeard for infrastructure. On 2026-10-21, Claude
Code 2.1.286 loaded both plugins into a clean configuration (ponytail 5.3.2,
neckbeard's Procfile candidate `a47a6f2`). Both session-start hooks and
ponytail's prompt hook completed successfully.

A live agent onboarded a Python fixture through analyze, plan, an Infracost
estimate, scaffold, and V0 validation. It edited only the profile and config;
all infrastructure and pipelines came from the CLI. No conflict between the
two plugins was observed. This was a local fixture test with confirmed choices
supplied up front, not an independent-user trial or a cloud deployment. See the
[pairing findings](docs/findings/2026-10-21-ponytail-pairing.md) for the checks,
tool restrictions, and background-validation friction. Other versions and
workloads remain unverified.

## Codex CLI

```sh
go install github.com/bealesh/neckbeard/cmd/neckbeard@latest
neckbeard skill install -target codex     # .codex/skills/neckbeard/
```

Invoke with `/skills` or `$neckbeard`. The installed skill carries the same
references the plugin ships (onboarding contract, release contract, schemas,
presets, catalog index).

## Cursor

```sh
go install github.com/bealesh/neckbeard/cmd/neckbeard@latest
neckbeard skill install -target cursor    # .cursor/skills/ + .cursor/commands/
```

Cursor loads the skill on demand and gets the plugin's commands as
`/neckbeard-analyze`, `/neckbeard-estimate`, `/neckbeard-sync`,
`/neckbeard-doctor`, `/neckbeard-help`, `/neckbeard-disco`.

## Any other SKILL.md-speaking agent

```sh
go install github.com/bealesh/neckbeard/cmd/neckbeard@latest
neckbeard skill install                   # .agents/skills/neckbeard/ — the open standard
neckbeard skill install -scope user       # once, for every repo on this machine
```

## No agent at all

```sh
go install github.com/bealesh/neckbeard/cmd/neckbeard@latest
neckbeard doctor -for plan   # bundled catalog check; no cloud access
neckbeard analyze            # draft profile + the questions code can't answer
$EDITOR app-profile.yaml     # resolve each assumption with a same-id confirmed answer
$EDITOR neckbeard.yaml       # cloud, region, tier, runtime, repo
neckbeard plan               # deterministic blueprint + topology summary
neckbeard estimate           # the money table, before anything exists
neckbeard scaffold           # infra/, pipelines, clusters/ — yours to review
neckbeard validate           # V0: fmt, validate, checkov, kubeconform
```

`neckbeard schema neckbeard`, `neckbeard schema app-profile`, and
`neckbeard presets` print the exact inputs. The
[onboarding contract](skills/neckbeard/references/onboarding.md) has a complete
config and examples for existing databases and unsupported dependencies.

## Optional tools

Planning and scaffolding need nothing beyond the CLI. The rest degrade
honestly: `neckbeard doctor` lists what is missing and `validate` reports
unavailable checks as NOT EXERCISED.

| Tool | Unlocks | Notes |
|---|---|---|
| OpenTofu (pinned version, see `neckbeard doctor`) | `validate` | `brew install opentofu` or the [release page](https://github.com/opentofu/opentofu/releases) |
| Infracost **0.10.x** + `infracost auth login` | `estimate` | The 0.10 binary from [GitHub releases](https://github.com/infracost/infracost/releases); the brew formula is the newer SaaS CLI, which neckbeard refuses |
| Checkov 3.3.10 | the policy gate in `validate` | `pipx install checkov==3.3.10` — skip lists are curated against this version |
| kubeconform | manifest validation on Kubernetes lanes | `go install github.com/yannh/kubeconform/cmd/kubeconform@v0.8.0` |
| actionlint | workflow lint on GitHub lanes | `go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` |

Doctor checks tool availability only — not cloud permissions, quotas, or
deployability.

## Release binaries

`scripts/install.sh` downloads `neckbeard_<version>_<os>_<arch>.tar.gz` from the
[GitHub Release](https://github.com/bealesh/neckbeard/releases) for the tag and
refuses any archive whose sha256 is not in that release's `checksums.txt`.
Releases are built by [`.github/workflows/release.yml`](.github/workflows/release.yml)
from the tagged commit, with no cloud credentials involved. If no release
exists for your platform it falls back to `go install`, and if Go is absent it
tells you so instead of guessing.
