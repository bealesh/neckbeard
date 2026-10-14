---
description: Install the neckbeard CLI if missing and report which prerequisites (OpenTofu, Infracost, Checkov, kubeconform, actionlint) are present and what each unlocks.
argument-hint: "[plan | estimate | validate | all]"
---

Check and, where safe, fix this machine's neckbeard prerequisites. Change
nothing in the repository.

1. If `neckbeard` is not on PATH, run the plugin's installer:
   `bash "${CLAUDE_PLUGIN_ROOT}/scripts/install.sh"`. If that placeholder is
   unfilled, use the plugin root the session-start context reported, or
   `go install github.com/bealesh/neckbeard/cmd/neckbeard@latest`. The
   installer verifies release binaries against the release's sha256
   checksums and falls back to `go install`; it never runs anything it did
   not verify. Report the installed version and path, and whether that
   directory is on PATH.
2. Run `neckbeard doctor -for ${ARGUMENTS:-all}` and present the result as a
   short table: tool, found/missing (with version), and what it unlocks —
   plan/scaffold need nothing beyond the CLI; `tofu` (pinned version) unlocks
   `validate`; `infracost` 0.10.x + its login unlocks `estimate`; `checkov`
   3.3.10, `kubeconform`, and `actionlint` unlock their validate gates and
   report NOT EXERCISED when absent.
3. For each missing tool, give me the exact install command for this OS. Do
   NOT install those tools yourself — they are the user's toolchain and
   Infracost needs an account login — unless I explicitly ask.

Doctor checks tool availability only; it says nothing about cloud
permissions, quotas, or deployability.
