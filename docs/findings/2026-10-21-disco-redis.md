# Disco Redis companion — 2026-10-21

## Scope and deployment decision

Redis is a first-class `needs` capability with provision/reference/undecided
modes. Provisioning is implemented only for the experimental Disco target.
The AWS/GCP/Azure catalog still refuses Redis provisioning; references remain
supported. Memcached remains a separate unsupported capability. Existing
profiles using resolved external Redis findings continue to render.

Disco daemon [v0.34.0 deployment flow](https://github.com/letsdiscodev/disco-daemon/blob/v0.34.0/disco/utils/deploymentflow.py)
starts replacement app services before removing old services. Its early conflict
handling is for published ports, not shared volumes. A Redis service added to
the app's `disco.json` would risk two Redis writers sharing the persistent
volume during an app redeploy. Merely setting a one-replica scale does not
prevent overlap between deployments.

Instead, the generated human-run runbook creates one long-lived Docker companion
on the same server. It joins the existing `disco-main` network under a
project-specific alias; the app receives its connection URL through Disco's env
store. App redeploys do not replace it. It has no published host port and requires
a random password held in a root-only host directory, outside Git and Docker
container arguments. Disco's Docker helpers create attachable overlay networks;
see [v0.34.0 Docker helper](https://github.com/letsdiscodev/disco-daemon/blob/v0.34.0/disco/utils/docker.py).
This is not a Disco Redis addon or managed Redis service.

The image is the official `redis:7.4.11-alpine`, pinned to multi-platform digest
`sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499`.
Defaults are a persistent `/data` volume, AOF with fsync every second,
`noeviction`, 256 MiB Redis maxmemory, and 512 MiB container memory. These defaults
do not promise no data loss or exactly-once queue delivery. Capacity, off-server
backups, restore, credential rotation, and planned image upgrades remain operator
responsibilities. Setup preserves matching containers/configuration and refuses
conflicting ownership/images or missing password configuration.

## Verification

Ran the generated runbook's setup block against local Docker 29.2.1. Adapted
only the root-user guard, host configuration path, and network name for the local
trial. The test used its own randomly named bridge network and volume, not a
running Disco server or Docker Swarm deployment. A peer container used the same
private DNS alias and a password supplied via environment rather than arguments.

Observed:

- Unauthenticated `PING` rejected with `NOAUTH`.
- Authenticated peer `PING` returned `PONG` using the generated hostname.
- No container port had a published host binding.
- Re-running setup preserved the container identity and credentials.
- `SET` data survived a graceful stop, removal, and recreation of only the Redis
  container against the same volume and configuration; authenticated `GET`
  returned the original value. This is persistence evidence, not a crash test.
- Trial container, volume, network, and temporary connection configuration were
  removed. No credentials were printed in the evidence.

Go tests exercise actual generated setup shell with Docker mocked, password
non-disclosure in output/arguments, private networking, profile generation,
custom connection bindings, duplicate/invalid bindings, Redis environment and
Compose detection, reference mode across all 12 cloud/VCS/runtime combinations,
and explicit refusal of cloud Redis provisioning. Full Go tests and vet passed.

## Not exercised

No new cloud resources were created. A real Disco app deployment using this
companion, the encrypted overlay network, app reconnect behavior, multi-node
placement, server replacement, backup/restore, crash durability, exhaustion,
and failover were NOT EXERCISED. No availability claim follows from the earlier
Postgres/Bellwether deployment run. The generated runbook documents explicit
companion removal and retained data/configuration; `disco projects:remove` alone
does not clean up these resources.
