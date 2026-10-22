package disco

import "strings"

// A companion container has a lifecycle independent of Disco's blue/green app
// deployments: two Redis writers must never mount the same data volume.
const redisImage = "redis:7.4.11-alpine@sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499"

func redisSetup(project string) string {
	return strings.NewReplacer("PROJECT", project, "REDIS_IMAGE", redisImage).Replace(`set -euo pipefail
[ "$(id -u)" = 0 ] || { echo 'Run on the Disco server as root.' >&2; exit 1; }
name=neckbeard-PROJECT-redis
volume=neckbeard-PROJECT-redis-data
config_dir=/var/lib/neckbeard/disco/PROJECT/redis
image=REDIS_IMAGE
check_config() {
  grep -Eq '^requirepass [0-9a-f]{64}$' "$config_dir/redis.conf" || {
    echo 'Redis password configuration is missing or invalid; recover it before starting.' >&2; exit 1;
  }
}
wait_ready() {
  for attempt in $(seq 1 30); do
    status=$(docker inspect -f '{{.State.Health.Status}}' "$name")
    if [ "$status" = healthy ]; then echo "Ready: $name (private port 6379, persistent $volume)"; return 0; fi
    if [ "$status" = unhealthy ]; then break; fi
    sleep 2
  done
  echo "Redis did not become healthy. Inspect $name; data and configuration were retained." >&2
  return 1
}
docker network inspect disco-main >/dev/null
if docker container inspect "$name" >/dev/null 2>&1; then
  [ "$(docker inspect -f '{{index .Config.Labels "io.neckbeard.redis.project"}}' "$name")" = PROJECT ] &&
  [ "$(docker inspect -f '{{.Config.Image}}' "$name")" = "$image" ] || {
    echo "Refusing an existing container with a different owner or image: $name" >&2; exit 1;
  }
  check_config
  docker start "$name" >/dev/null
  wait_ready
  exit 0
fi
if docker volume inspect "$volume" >/dev/null 2>&1; then
  [ "$(docker volume inspect -f '{{index .Labels "io.neckbeard.redis.project"}}' "$volume")" = PROJECT ] || {
    echo "Refusing a volume owned elsewhere: $volume" >&2; exit 1;
  }
  [ -f "$config_dir/redis.conf" ] || {
    echo 'Existing data has no connection configuration. Recover it before proceeding.' >&2; exit 1;
  }
fi
umask 077
mkdir -p "$config_dir"
chmod 700 "$config_dir"
if [ ! -e "$config_dir/redis.conf" ]; then
  password=$(openssl rand -hex 32)
  # Only a random hex password is interpolated into this Redis config.
  printf 'requirepass %s\nappendonly yes\nappendfsync everysec\nmaxmemory 256mb\nmaxmemory-policy noeviction\n' "$password" > "$config_dir/.redis.conf.tmp"
  mv "$config_dir/.redis.conf.tmp" "$config_dir/redis.conf"
  unset password
fi
# The container's unprivileged Redis user reads this single bind-mounted file;
# its parent directory remains root-only on the host.
check_config
chmod 444 "$config_dir/redis.conf"
docker volume create --label io.neckbeard.redis.project=PROJECT "$volume" >/dev/null
docker run -d --name "$name" --restart unless-stopped \
  --label io.neckbeard.redis.project=PROJECT \
  --memory 512m --memory-swap 512m \
  --network disco-main --network-alias PROJECT-redis \
  --mount "type=volume,source=$volume,target=/data" \
  --mount "type=bind,source=$config_dir/redis.conf,target=/usr/local/etc/redis/redis.conf,readonly" \
  --health-cmd 'REDISCLI_AUTH="$(sed -n "s/^requirepass //p" /usr/local/etc/redis/redis.conf)" redis-cli ping | grep -qx PONG' \
  --health-interval 5s --health-timeout 3s --health-retries 12 \
  "$image" redis-server /usr/local/etc/redis/redis.conf >/dev/null
wait_ready
`)
}

func redisRunbook(project, env string) string {
	return strings.NewReplacer("PROJECT", project, "ENV_NAME", env, "SETUP_SCRIPT", redisSetup(project)).Replace(`Redis runs as one authenticated companion container on the Disco server,
separate from the app's blue/green lifecycle. It joins Disco's private network
as PROJECT-redis:6379; no host port is published. It is not a Disco addon and
is not removed by ` + "`disco projects:remove`" + `. The server's root operator manages it.

From your local shell, with SERVER_IP still set, run:

` + "```sh" + `
ssh "root@$SERVER_IP" 'bash -se' <<'NECKBEARD_REDIS'
SETUP_SCRIPTNECKBEARD_REDIS
` + "```" + `

Then set the app connection variable locally without printing the value:

` + "```sh" + `
REDIS_PASSWORD="$(ssh "root@$SERVER_IP" "sed -n 's/^requirepass //p' /var/lib/neckbeard/disco/PROJECT/redis/redis.conf")"
[ -n "$REDIS_PASSWORD" ] || { echo 'Redis password unavailable' >&2; exit 1; }
disco env:set --project PROJECT ENV_NAME="redis://:$REDIS_PASSWORD@PROJECT-redis:6379/0"
unset REDIS_PASSWORD
` + "```" + `

- Default: 256 MiB Redis maxmemory, 512 MiB container memory, no eviction,
  AOF with fsync every second. Review sizing before use; a full instance returns
  write errors. A crash can lose recent writes; this is not exactly-once queue
  delivery or managed high availability. Apps must reconnect/retry during startup.
- The password stays in a root-only host directory, outside the repository.
  Disco stores the app connection variable. Project traffic uses Disco's shared
  network; Redis authentication is required and Redis itself does not use TLS.
  This is a single trusted server, not a per-project network isolation guarantee.
- App redeploys leave Redis running. Keep exactly one Redis container attached
  to this volume; never add a Redis service sharing it to disco.json.
- Back up /data using a Redis-consistent backup method, and keep the host
  connection configuration separately. A volume/AOF is not an off-server backup.
  Restore and failover have not been verified by neckbeard.
- Re-running setup preserves an existing matching container. It does not upgrade
  its image or change its password. For a planned upgrade, take a verified backup,
  stop and remove only this container, then recreate it against the same volume
  and configuration; never run old and new Redis against the data simultaneously.
- After stopping/removing the app, remove the companion with the command below.
  This retains the data volume and connection configuration intentionally.

` + "```sh" + `
ssh "root@$SERVER_IP" 'docker stop neckbeard-PROJECT-redis && docker rm neckbeard-PROJECT-redis'
` + "```" + `

Only after backup/retention review, explicitly delete volume
` + "`neckbeard-PROJECT-redis-data`" + ` and directory
` + "`/var/lib/neckbeard/disco/PROJECT/redis`" + ` on the server. Project deletion alone
leaves these resources behind.
`)
}
