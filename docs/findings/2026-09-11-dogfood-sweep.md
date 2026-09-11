# Findings: analyzer dogfood sweep against real OSS applications

Date: 2026-09-11. First time the analyzer met codebases it wasn't developed
against. Five popular open-source applications, chosen to stress different
stacks and to include workloads that *should* be refused: outline (Node/Koa),
plausible (Elixir/Phoenix), saleor (Django), mastodon (Rails, genuinely
multi-image), miniflux (Go, distro packaging only). Everything below ran the
installed-binary flow with no cloud credentials: analyze → resolve → plan →
scaffold → V0 validate.

## What broke, and the fixes

1. **Devcontainers became draft services.** mastodon's and saleor's
   `.devcontainer/Dockerfile` each produced a service named `devcontainer`
   listening on 8080. Fix: dot-directories are tooling by convention and are
   now skipped entirely.
2. **Base and packaging images became services.** outline's `Dockerfile.base`
   became service "base"; miniflux's `packaging/*/Dockerfile` became services
   "debian" and "rpm". Fix: when a repository-root `Dockerfile` exists it is
   the application image, and every other candidate becomes a **plan-gating
   assumption** (`container-images`) naming them — planning stays blocked until
   the agent confirms none is a separately deployed service. mastodon's real
   second image (`streaming/Dockerfile`) exercised the other side: adding it
   back as a service produces the named multi-image refusal, not a force-fit.
   With no root Dockerfile (miniflux) there is no convention to lean on, so all
   candidates stay visible and the workload gate refuses honestly.
3. **Typed env wrappers defeated every accessor pattern.** outline reads
   `env.REDIS_URL` through a typed environment class, hiding postgres, S3, and
   redis entirely. Fix: an `env.NAME` member-access pattern (its captures land
   in the same evidence-backed classes as before).
4. **Env templates were ignored.** outline declares its environment surface in
   `.env.sample`; only `.env.example` was scanned, and only for `$VAR` reads.
   Fix: `.env.example/.sample/.template/.dist` files are scanned for
   `NAME=` declarations. Real `.env` files are never read (tested).
5. **Config-helper indirection hid PostgreSQL.** plausible reads
   `"DATABASE_URL"` through `get_var_from_path_or_env`. Fix: the exact quoted
   literal `"DATABASE_URL"` now produces an *inference* (medium confidence,
   "verify in the code") plus an undecided need — never a fact claiming a
   direct read that wasn't observed.
6. **One capability, five findings.** mastodon's `REDIS_DRIVER`,
   `REDIS_NAMESPACE`, … each produced its own unsupported entry. Fix: one
   finding per capability, naming every variable, evidence merged.
7. **Missing dependency classes.** mastodon's Elasticsearch (`ES_*`,
   `ELASTIC*`, `OPENSEARCH*`) and plausible's ClickHouse now classify as
   unsupported findings instead of vanishing.
8. **`/_health` wasn't in the health-path vocabulary.** outline's actual
   endpoint (server/main.ts). Added.
9. **The `queue: max` actionlint gap hit V0 for real** (launch-plan Gate 1):
   any user with actionlint installed got a FAILED V0 on generated workflows.
   The validator now runs actionlint with JSON output and excludes exactly one
   diagnostic — the upstream-missing `queue` key
   (github.com/rhysd/actionlint/issues/680) — and only when the flagged line
   literally reads `queue: max`. Anything else on that line, any other
   diagnostic, or unparseable output stays a failure, and the report states
   the exclusion count and reason. Unit-tested in both directions.

## Honest misses that remain (by design, documented)

- **saleor's PostgreSQL is invisible to deterministic detection**: the app
  never writes the literal `"DATABASE_URL"`; it goes entirely through
  `dj_database_url.DEFAULT_ENV`. The draft says nothing about postgres. This is
  exactly the division of labor: the agent reading `settings.py` catches it in
  the refine step, and the skill instructs that reading. A future
  dependency-manifest heuristic (dj-database-url / django-environ in
  requirements) could add an inference; deliberately not guessed at today.
- Framework-internal env reads in general (Rails credentials, Django settings
  wrappers) surface only when a literal or template names them.

## Sweep results

| App | Lane exercised | Outcome |
|---|---|---|
| outline | aws / github / serverless | Full V0 green; postgres + object-storage + secrets detected, redis external-disposition |
| plausible | gcp / gitlab / serverless | Full V0 green (24 checks); postgres via literal inference, ClickHouse named unsupported |
| saleor | analyze/refine only | Draft: storage + queues + redis correct; postgres documented miss (above) |
| mastodon | refusal path | Root-preference gates; restored streaming service → named multi-image refusal |
| miniflux | refusal path | No root Dockerfile; packaging candidates flagged, workload gate refuses |

V1/V2 were NOT EXERCISED throughout — this sweep proves detection and static
generation against unfamiliar code, not deployability.

## Addendum: live skill-following agent test (same day)

A fresh agent session was given only the installed `.agents/skills/neckbeard/`
skill and the saleor checkout — no other neckbeard knowledge — and asked to
onboard the app. It caught the dj_database_url postgres miss with file:line
evidence (settings.py:136), corrected the health path to `/health/`
(asgi/__init__.py:40), added the Celery worker and beat-scheduler services from
pyproject.toml, gave redis/queues external dispositions, and produced a valid
blueprint on its first `plan`. The division of labor held.

Its friction report drove these fixes: resolved assumptions now print as
`decision:` lines and live in the blueprint's `decisions` field (warnings are
reserved for open risks); the app-profile schema now permits `secret_name` on
provision-mode needs (the planner honored it for postgres's connection binding
while the schema forbade it); and the skill/onboarding docs gained the
root-Dockerfile contract, first-class datastore review ("a web framework with
no database in its draft is a detection gap, not a stateless app"), scheduler-
singleton guidance, disposition-vs-confirmed clarification, the
provisioned-capability env-mapping contract (including the honest note that
object-storage variable wiring is manual today), the `doctor -for estimate`
step in the flow diagram, and a public DESIGN.md link for section references.
