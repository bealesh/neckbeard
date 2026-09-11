# Deployment lifecycle: the release workflow contract

This continues [onboarding.md](onboarding.md) after `scaffold` + `validate`
are green and the human has completed the bootstrap runbook the scaffold wrote
(`docs/bootstrap.md` in their repo — elevated credentials, one time per
environment; that step is theirs, never yours).

**Trust boundary, first.** You prepare *intent* — reviewable files the user
commits. Deployments execute through the generated CI jobs under their OIDC
identities and environment gates; production additionally requires the
platform's approval rule, which `configure-ci -env prd` refuses to configure
around. Running `release deploy` from a workstation is possible with operator
credentials but is the exception (first-time setup, break-glass), not the flow
you drive. Never paste or request secret values; the tooling moves them
cloud-to-cloud without you ever seeing one.

**Honesty note.** Deploy → update → rollback has passed live on AWS and Azure
GitHub/serverless (with real database persistence and failure recovery);
promotion, GitLab lanes, and Kubernetes are implemented but not yet
lifecycle-proven. Say so when the user asks how tested their lane is; the
[launch plan](https://github.com/bealesh/neckbeard/blob/main/docs/launch-plan.md)
tracks acceptance per path.

## Order of operations (per environment, dev first)

```sh
neckbeard release foundation-plan -env dev    # network/registry/secrets/storage (+cluster on k8s)
# The HUMAN reviews the saved plan (creates only; replacements/deletes refused).
neckbeard release foundation-apply -env dev
neckbeard release prepare-database -env dev   # only when a postgres need is provisioned
neckbeard release configure-ci -env dev       # publishes non-secret CI variables + the receipt store
git add .neckbeard/deploy/dev.store.json      # commit with the backend config
```

- `prepare-database` stores a generated password in the cloud connection
  secret *before* creating the database, so retries reuse it; a failed setup
  leaves a `pending.invalid` connection the runners refuse to deploy. One
  environment at a time. An existing connection pointing elsewhere is refused,
  never overwritten — migrating a real database is an explicit operation.
- Foundations must exist before the first image build (the registry) and the
  database/secrets before the first full infrastructure apply.

## First image → first release

Application CI builds, scans, and pushes the image, then publishes
`built-image.txt` containing the immutable digest. From a successful build:

```sh
neckbeard release pin -env dev -image "$(cat built-image.txt)" \
  -health-url https://dev.example.com/healthz
git add releases/ && git commit   # release intent is reviewed and committed
```

Images are always `repo@sha256:…` digests — a tag is refused. HTTP services
require an HTTPS health URL before any runtime change. For the bellwether test
app, `-expected-version`, `-check-environment`, and `-require-database` deepen
verification; for arbitrary apps only the 200-over-HTTPS check applies unless
their health endpoint speaks the same JSON.

Then trigger the release job: GitHub — the `neckbeard-release` workflow's
environment selector; GitLab — a main-branch pipeline with
`NECKBEARD_RELEASE_ENV=<env>`, then its manual job. The job restores receipts,
stages the digest, deploys via the native runtime API, verifies the *running*
digest and health, and saves a verified receipt pair to cloud storage. CI
serializes releases per environment; don't run concurrent manual deployments
against the same environment.

## Promote and roll back

```sh
neckbeard release restore-receipts -env dev     # on any fresh checkout
neckbeard release promote -from dev -env stg -health-url https://stg…/healthz
# review + commit releases/, then trigger the stg release job
neckbeard release rollback -env stg             # targets the previous verified receipt
```

Promotion order is dev → stg → prd, enforced; it copies the verified digest
(never rebuilds) and requires a verified source receipt. Rollback after a
*failed* update targets the still-current last-good receipt — the failed
attempt never replaced it. `restore-receipts` treats storage errors as errors:
missing remote receipts with local history present means investigate, not
start over.

## When something fails

Consult the findings ledger first:
https://github.com/bealesh/neckbeard/tree/main/docs/findings — known classes
(Azure async deletes eating same-named recreations, orphaned resources needing
import-or-fail, regional capability holes) live there with recoveries. Rules
that always hold: never clear a state lock without confirming the owning run
and that all writers stopped; a canceled apply means re-plan and read the diff
before anything else; capture new failure classes as findings before patching
around them.
