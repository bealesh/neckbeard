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

## Release gate

[Gate 5 of the launch plan](launch-plan.md#gate-5--independent-users-and-release-candidate)
is the source of truth for release acceptance. Keep candidate commit, package
checksums, and verification results tied together. The release packaging commit sets the manifest to `0.4.0-beta.1`. Tag that
reviewed commit and build/test the exact tagged source before publishing.
The [release notes](releases/v0.4.0-beta.1.md) describe the delivered scope
and outstanding validation.

## Beta installation after publication

`go install ...@latest` selects the stable release. The plugin installer
defaults to its checkout's manifest version; the beta checkout selects v0.4.0-beta.1. Beta testers can also choose this
version explicitly after publication:

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

The three independent-user trials in Gate 5 are still outstanding and tracked
in [issue #28](https://github.com/bealesh/neckbeard/issues/28). Agent-driven
fixture checks do not count toward that gate.

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
