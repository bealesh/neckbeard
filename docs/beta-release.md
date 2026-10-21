# v0.4.0-beta.1 candidate

This is a release preparation record, not an announcement that the beta is
published. The intended beta product is app onboarding: analyze, plan, estimate,
scaffold, validate, and the human-run bootstrap guide. Cloud deployment lifecycle
features and the separate Disco target remain experimental.

## Included

- Disco rendering, `/neckbeard:disco`, bundled agent guidance, and safe output
  preflight. The October 20 Bellwether live test passed initial deployment,
  Postgres and worker checks, scheduled execution, an update with preserved data,
  and verified cleanup. GitHub push deployment, rollback, and restore were not
  exercised by that test.
- Procfile process-role detection. Commands stay as cited evidence rather than
  executable container arguments. Development files are labeled, production
  Procfiles take precedence, and release/migration hooks are excluded.
- Verified ponytail pairing, described in INSTALL.md and the findings ledger.
- Beta/RC tags are published as GitHub prereleases, keeping stable releases
  separate. A credential-free test checks the actual publishing command.

## Candidate checks

Before publishing, record the exact candidate commit, package checksums, and
results for:

- The normal Go, plugin, security-invariant, and cloud static CI checks.
- Linux and macOS archives on amd64 and arm64; run native-platform archives and
  state explicitly which cross-built binaries were not executed.
- Fresh installation, explicit upgrade from v0.3.0, checksum rejection, and the
  embedded Codex/Cursor skills, references, and commands.
- Preservation of user edits when regenerating after an upgrade.

After the source is accepted, tag that exact commit and verify the downloaded
artifacts, not just local builds. Do not mark this release as stable.

## Beta installation after publication

The default installer and `go install ...@latest` continue to select a stable
release. Beta testers must choose this version explicitly:

```sh
bash scripts/install.sh -v v0.4.0-beta.1 -f
```

Run that script from the matching plugin checkout. For a source installation:

```sh
go install github.com/bealesh/neckbeard/cmd/neckbeard@v0.4.0-beta.1
neckbeard skill install -target codex
```

Use `-target cursor` for Cursor. Reinstall the skill after upgrading the binary
so commands and references match. Claude Code users need both the beta plugin
revision and the beta CLI; updating one does not update the other.

## Independent-user trial

Three independent users still need to complete onboarding on their own supported
applications. Agent-driven fixture checks do not count as independent users.
Each trial should record the installed version, app/stack, steps completed,
confusing questions, manual repairs, refusals, and validation results. Findings
belong in issue #28; unsupported needs should remain explicit rather than expand
the beta workload contract.

## Draft release notes

This beta adds Disco as an experimental self-hosted target and improves process
role detection from Procfiles. The plugin's supported onboarding flow generates
reviewable infrastructure, costs, pipelines, and validation results; cloud
deployment, promotion, and recovery remain experimental. Disco uses its own
profile-to-config flow with documented cost, overwrite, and validation limits.

No automatic migration is required. Keep refined profiles and configuration,
upgrade the CLI and installed skill together, then re-plan and review scaffold
conflicts. Never discard a user edit just to make regeneration pass. See the
README and Disco reference for supported workloads and known limitations.
