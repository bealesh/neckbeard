# Launch plan

Status: proposed execution plan, 2026-09-10. The deployment checkpoint is not a
release candidate. This plan preserves the launch requirement of **AWS, GCP and
Azure × GitHub and GitLab × serverless containers and Kubernetes: 12 paths**.

Implementation checkpoint: `aa127b7` on `feat/tranche-2-deployment`. The checkpoints
below describe completion criteria; recording this plan does not mark them done.

## Starting point

| Work | Evidence today | Still required |
|---|---|---|
| Tranche 1: onboarding | Merged in #5; portable binary/catalog, evidence-backed analysis, deterministic planning, safe generation, estimates, unrelated-app fixtures | Teach the installed agent the completed release workflow and verify it from a fresh installation |
| Tranche 2: deployment | AWS/GitHub and Azure/GitHub serverless deployment, update and rollback passed with real database and worker checks; AWS failed-verification recovery passed | Promotion, GCP, GitLab app deployments, Kubernetes, production boundaries and a repeatable full matrix |
| Tranche 3: operations | Application receipts and some recovery behavior exist | Database restore drills, interruption recovery, cost controls and verified cleanup |
| Tranche 4: launch | Source distribution and installation paths exist | Independent users, a representative real app, consistent documentation and tested release artifacts |

Full lifecycle acceptance is **0/12**. Two paths have substantial live evidence;
neither has completed every release gate. See [deployment evidence](tranche-2.md)
and the [checkpoint](findings/2026-09-09-tranche-2-checkpoint.md).

The first supported workload remains the existing onboarding contract: a
containerized application within the catalog's pinned topology, using the adopt
path in existing cloud accounts/projects/subscriptions. Unsupported needs remain
explicit findings. Reconcile DESIGN's founding-path milestone with that contract
before publishing launch claims; landing zones must not be advertised as built.

## Gate 1 — Integrate the checkpoint and make tests repeatable

Finish this before expanding the live deployment matrix.

- [ ] Review the checkpoint through a PR, preserving the distinction between
  implemented code and accepted deployment paths. Resolve review findings before
  merge; do not label the checkpoint tranche-two completion.
- [ ] Extend the installed skill and onboarding guide through foundation setup,
  database preparation, image publication, release, promotion and rollback. Keep
  examples usable outside a neckbeard source checkout.
- [ ] Resolve `queue: max` compatibility in the normal validator. Check valid
  queue settings without suppressing unrelated workflow errors, explain any
  linter limitation in the report, and exercise the installed linter in CI.
  Our temporary validation-script exception is not a shipped fix.
- [ ] Build the live harness around the existing release runner and application
  probes. Use a small explicit fixture manifest, not a second deployment engine.
  Record cloud/VCS/runtime, CLI/catalog version, commit, image digest, resource
  inventory, run IDs, assertions and cleanup result. Keep credentials and raw
  secret-bearing state out of evidence artifacts.
- [ ] Make one AWS and one Azure serverless fixture pass from fresh disposable
  resources through deployment, update, rollback and cleanup using generated
  paths. Convert every necessary manual repair into a catalog fix or a documented,
  repeatable bootstrap prerequisite, then rerun from scratch.

**Exit:** a clean installation follows the shipped guide; normal validation has
no unexplained failures; those two fixtures can be recreated and removed by the
harness without undocumented intervention. Fresh tests do not reuse repaired
resources as evidence of successful first-run setup.

## Gate 2 — Complete all six serverless paths

Suggested order: GCP/GitHub, AWS/GitLab, GCP/GitLab, Azure/GitLab, then repeat the
complete six-path suite against the same candidate. Reuse the shared runner;
retain only provider/VCS differences that live tests demonstrate.

- [ ] Apply the reviewed GCP bootstrap correction only after the pending explicit
  Cloud Run Admin approval; prove HTTPS, secret access, database connectivity,
  worker effects and report execution through generated CI.
- [ ] Move GitLab beyond identity preflight: build and scan images, run actual
  infrastructure and app releases, and persist verified receipts on each cloud.
- [ ] Run dev → stg → prd promotion without rebuilding. Verify the same image
  digest, the target environment identity, target database isolation, and registry
  access. Roll back from cloud-restored receipts in a fresh checkout.
- [ ] Verify both production approval mechanisms promised in DESIGN: platform
  environment reviewers and the protected-ref fallback for repositories without
  that feature. Bind cloud trust to the approved ref/environment as appropriate.
  A public GitHub gate-only fixture tests the platform feature; it does not prove
  the private-repository product path works.
- [ ] Exercise denied branches, refs and environments; prove plan identities
  cannot apply, write state or read secret values. Resolve GitLab's subject-claim
  limitation for AWS/Azure with an enforceable boundary and negative tests before
  claiming exact environment isolation.
- [ ] Test scheduled invocation through each cloud scheduler, in addition to the
  already available direct report-job probes. Restore any temporary test schedule.
- [ ] Exercise an infrastructure change and safe regeneration after native image
  updates. Verify no surprise replacement, image reversion, or overwritten user
  edits. Test multiple queued infra/release jobs, not only sequential dispatch.

**Exit:** all six serverless paths pass the common lifecycle defined below from
fresh fixtures. Production runs include a permitted approval and denied bypasses;
an identity preflight alone does not satisfy the deployment test.

## Gate 3 — Complete all six Kubernetes paths

Finish the complete Kubernetes path on one cloud first, then apply the same
harness contract to the other clouds and both VCSs. Keep environment isolation;
do not share clusters merely to make the tests pass.

- [ ] Finish generated EKS/GKE/AKS bootstrap, CI cluster access and private
  repository credentials. Confirm the prerequisites and permissions before apply.
- [ ] Establish HTTPS, application secrets and private database connectivity.
  Verify HTTP identity, worker effects and scheduled jobs in the cluster.
- [ ] Prove Flux follows the approved commit. Merging an unapproved application
  change must not advance production, including during bootstrap reconciliation.
- [ ] Run immutable promotion and rollback, infrastructure changes, regeneration
  and teardown with the same assertions as serverless paths.
- [ ] Stagger cluster tests within an explicit cost/quota envelope; clean up each
  completed fixture and preserve resumable inventories for interrupted runs.

**Exit:** **12/12 paths** pass the deployment lifecycle against the same candidate.
Static provider validation and valid Kubernetes manifests remain separate checks.
This completes tranche two, subject to the operational release gates below.

## Gate 4 — Prove operations and recovery

This is tranche three. Some harness plumbing and cleanup work starts in gate 1;
the complete drills run after the deployment paths are reliable.

- [ ] Restore actual PostgreSQL data on every cloud for each recovery configuration
  we advertise. Measure recovered data age and elapsed recovery time against the
  claimed RPO/RTO; backup settings and application rollback do not prove this.
- [ ] Exercise failed apply, canceled CI, expired login, failed health checks,
  interrupted receipt writes and retry from a new checkout. Preserve the last
  verified release and detect partial setup instead of silently starting over.
- [ ] Provide the operator's interrupted-run procedure: identify the owning run,
  verify writers have stopped, recover state/locks, re-plan and resume. Never
  automatically clear an unexplained lock.
- [ ] Complete cost reporting for the actual launch topology: idle baseline,
  assumptions, unpriced resources and provenance. Deliver and test promised budget
  alerts; describe them accurately as notifications, not spending caps.
- [ ] Verify teardown and residual-resource detection after success and failure,
  including state storage, CI identities, registry images, DNS/certificates and
  resources created by managed services. Preserve user-owned resources and keep
  state until cleanup is verified.

**Exit:** recovery assertions pass, costs are understandable, and the harness
leaves no unexplained billable resources or active temporary credentials. A failed
cleanup leaves an actionable inventory and blocks a passing release verdict.

## Gate 5 — Independent users and release candidate

This is tranche four. Documentation can improve throughout; the final trial must
use the artifacts intended for release.

- [ ] Test at least three people who did not implement neckbeard, using their own
  supported apps and the shipped guidance. Cover both VCSs and both runtimes
  across these trials; the automated matrix covers every combination.
- [ ] Deploy and update one representative real application beyond Bellwether.
  Record confusing prompts, manual repairs and unsupported needs; fix supported
  flows instead of expanding the workload contract during launch preparation.
- [ ] Align README, DESIGN, agent guidance, commands and support table. Publish
  verified coverage and account-plan prerequisites; remove stale placeholder and
  deployment claims. State the tested topology and known limitations explicitly.
- [ ] Test versioned release artifacts and supported installation paths from
  clean machines/checkouts, including bundled references, upgrade/regeneration,
  migration notes, and recovery guidance.
- [ ] Repository settings, partially applied 2026-09-11: default workflow token
  read-only with Actions PR-approval disabled; fork-PR workflow approval
  required for **all** outside collaborators; an active `main` ruleset requiring
  PR-only squash merges and the `go`/`tofu`/`fork-safety` checks (no deletions
  or force pushes). Still gated on going public: secret scanning with push
  protection, private vulnerability reporting, and raising the ruleset's
  required approvals once there is more than one maintainer. The
  credential-free CI invariant (CONTRIBUTING.md) is enforced by the
  `fork-safety` job regardless of visibility.
- [ ] Run the full release matrix and recovery suite on the tagged candidate.
  Keep evidence tied to that exact commit/catalog. Rerun affected checks after
  changes, and run the final full suite again before publishing.

**Exit:** independent users complete the supported journey without unpublished
instructions, all required checks pass, cleanup is accounted for, review findings
are resolved, and release artifacts match the tested candidate. Then publish.

## Common lifecycle for every matrix path

1. Start with a recorded, fresh disposable fixture and complete supported bootstrap.
2. Build and scan one immutable image; deploy the HTTP service, worker, scheduled
   job, database and any other resources claimed by the fixture.
3. Verify HTTPS, running image/commit identity, environment identity, real database
   access, worker effects and scheduled execution.
4. Update the application; retain an original record and process a fresh one.
5. Promote the same digest across isolated environments with production approval
   enforced, then restore receipts and roll back from a fresh checkout.
6. Change infrastructure and regenerate; preserve native release ownership and
   user edits. Verify expected changes and reject unexpected destructive drift.
7. Exercise failure/retry behavior and the required access-denial cases.
8. Tear down owned resources and check for leaks. Record PASSED, FAILED or NOT
   EXERCISED for each assertion; skipped or manually repaired runs cannot pass.

## Immediate execution order

| Next work package | Deliverable | Dependency |
|---|---|---|
| 1. Checkpoint review | Reviewed deployment changes and explicit known-gap list | This commit and plan |
| 2. Installed workflow | Release guidance, normal validator compatibility, CI lint coverage | Checkpoint review |
| 3. Harness baseline | Repeatable fresh AWS/Azure fixtures, evidence and cleanup | Installed workflow |
| 4. Remaining serverless paths | GCP and GitLab deployments; promotion and both production gates | Harness; reviewed permissions and repository setup |
| 5. Kubernetes matrix | Cluster/Flux/HTTPS bootstrap and six accepted paths | Shared lifecycle harness |
| 6. Operational drills | Restore measurements, interruption recovery, costs and leak detection | Deployable paths; can progress per completed cloud |
| 7. Release candidate | Independent-user results, real-app dogfood and exact-candidate evidence | All launch gates |

Pending decisions are execution prerequisites, not approvals granted by this plan:

- The existing reviewed GCP project-level Cloud Run Admin grant remains unapplied
  after automatic approval review requested explicit authorization.
- Publication of the prepared public GitHub gate-only fixture remains unapproved.
  Keep it private locally until permission arrives; do not change an existing
  private application repository's visibility.
- Review additional bootstrap plans, account quotas and expected test costs before
  expanding disposable infrastructure. Do not infer that earlier approvals cover
  a different privilege grant or account.

## Scheduling and progress reporting

Use these exit gates to schedule work; a count of files or generated combinations
is not a launch readiness measure. Re-estimate remaining work after the first
clean GCP deployment, first real GitLab deployment, and first complete Kubernetes
path. Those are the largest untested integrations today. Set a public launch date
after those uncertainties are resolved and the operational drills are passing.

For every work package, report the candidate commit, checks actually passed,
unexercised assertions, blockers and outstanding resource cleanup. Feature scope
stays fixed during launch preparation; new capabilities go to the post-launch
backlog unless required to fulfill the existing supported contract.
