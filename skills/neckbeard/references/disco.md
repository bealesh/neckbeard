# Disco render target (experimental)

Use this path when the user chooses [Disco](https://disco.cloud), a self-hosted
PaaS on an Ubuntu server. It renders a reviewed `app-profile.yaml` directly,
not a cloud/runtime value in `neckbeard.yaml`.

## Flow

1. Run `neckbeard analyze` and refine the profile using the onboarding contract.
   Resolve assumptions and unsupported findings with the user's answers.
   Skip cloud, VCS, tier, and region configuration: this path does not consume
   `neckbeard.yaml` or `blueprint.yaml`.
2. Confirm the Disco project name: lowercase letters, digits and dashes,
   starting with a letter, at most 42 characters.
3. Explain the server's cost and operating responsibilities before generation.
   The user supplies the server; neckbeard does not provision it. No Infracost
   report, budget alert, or monthly ceiling is implemented for Disco. Do not
   present a cloud estimate as the cost of this path.
4. From the repository root, run:

   ```sh
   neckbeard disco -project myapp
   ```

5. Review `disco.json` and `docs/disco.md`. Existing outputs are refused;
   `-force` replaces both files, including user edits. Review changes with the
   user before overwriting. There is no `.neckbeard-new` conflict workflow.
6. Give the runbook to the human operator. They install the Disco CLI, initialize
   or select the server, create the project and datastores, set secret values, and
   deploy. The agent prepares files; it never runs these credentialed steps or
   reads secret values. GitHub is linked after datastores and secrets are ready;
   local-directory deployment is also supported.

## Supported shape and limits

- One Dockerfile, built with the repository root as its build context; exactly
  one HTTP service, plus optional workers and cron jobs sharing that image.
- HTTP maps to Disco's reserved `web` service. Non-HTTP services cannot be named
  `web`. Cron jobs require an explicit command and a five-field schedule in UTC.
- Provisioned Postgres uses Disco's addon on the same server. Its disk, backups,
  recovery, and capacity are the operator's responsibility; this is not managed
  database availability. Existing services use their named environment variables.
- Provisioned Redis runs as an authenticated Docker companion on the same
  trusted server, attached to `disco-main` without a published host port. It is
  separate from `disco.json`: Disco's blue/green app deployments must not run
  two Redis writers against the same volume. The generated runbook provisions
  it and binds `REDIS_URL` (or `secret_name`) through Disco's encrypted env store.
  Defaults are 256 MiB maxmemory, 512 MiB container memory, `noeviction`, and AOF
  fsync every second. Review capacity and data-loss tolerance; this is not HA,
  a spending cap, or an exactly-once queue. Apps must retry initial connections.
  App/project deletion does not clean up this companion, its volume, or its
  root-only host configuration; follow the runbook's explicit cleanup steps.
  Backups, recovery, upgrades, and server capacity remain operator responsibilities.
- Provisioned object storage has no mapping and is refused. External unsupported
  dependencies require an explicit disposition and a connection variable.
- The HTTP health path is documented for external checks. Disco's port readiness
  check does not prove that endpoint works; no in-container health command is
  generated. Database migrations are not generated either.
- `plan`, `estimate`, `scaffold`, `validate`, and the cloud release workflow do not
  validate or operate these Disco outputs. Do not claim their V0/V1/V2 results
  for this path.

## Evidence

PR #38 reports fresh Ubuntu 24.04 DigitalOcean runs with Disco daemon 0.34.0 and
CLI 0.5.72: HTTPS, Postgres, worker effects, cron execution, an application update
with preserved data, and verified resource teardown. These are Bellwether tests,
not proof that the user's app deploys. GitHub push deployment, other VPS
providers, rollback, and database recovery were not exercised by that lane.

## Redis profile

Redis detection produces an undecided need, not an unsupported finding. Confirm
whether it is a production dependency, provision it on Disco or reference an
existing instance, and review its memory, persistence, eviction, and recovery
requirements before generating. Memcached remains unsupported; detecting Valkey
requires confirming Redis compatibility before provisioning the pinned Redis image.
For old profiles, replace a resolved Redis unsupported entry with a need and
carry its evidence over; do not leave contradictory Redis entries in both lists.

```yaml
needs:
  - capability: redis
    mode: provision
    secret_name: REDIS_URL
```

`secret_name` defaults to `REDIS_URL` for provisioned Redis. Only one provisioned
Redis instance per project is supported. A reference needs an explicit connection
variable. AWS/GCP/Azure Redis provisioning remains unavailable; choose reference
mode there. No Redis availability or restore claim follows from the earlier
Postgres/Bellwether live evidence above.

The [Redis companion findings](https://github.com/bealesh/neckbeard/blob/main/docs/findings/2026-10-21-disco-redis.md)
record the deployment-lifecycle decision and local Docker verification separately
from the earlier live Postgres evidence.
