Written to `PRODUCTION_NEEDS.md` in the app directory. Summary of what it documents, each backed by file:line in the file:

**Processes.** One long-running process only: the Next.js standalone server started by `sh scripts/start-docker.sh`, which runs two one-shot scripts then `exec`s `node server.js` (`Dockerfile:104`, `scripts/start-docker.sh:14-16`). There is no worker, no cron, no scheduler, and no queue consumer anywhere in server code. The `@umami/mcp` stdio CLI exists but is a client tool, not part of the deployment.

**Port and health.** Listens on 3000 bound to 0.0.0.0 via `PORT`/`HOSTNAME` (`Dockerfile:99-102`). Health check is `GET /api/heartbeat` (`src/app/api/heartbeat/route.ts:1-3`), the same path the project's own compose files use. It does not touch the database.

**Datastores.**
- PostgreSQL is required, found via `DATABASE_URL`, minimum 9.4, with optional `DATABASE_REPLICA_URL` read replica and `DIRECT_DATABASE_URL` for migrations.
- ClickHouse is optional via `CLICKHOUSE_URL`. Its schema is not applied by the app; the operator must apply `db/clickhouse/schema.sql` and migrations.
- Redis is optional via `REDIS_URL` for website/session caching and server-side auth tokens. SSO returns a 500 without it.
- Kafka is optional via `KAFKA_URL` plus `KAFKA_BROKER`, producer only, and only used when ClickHouse is also on. No consumer exists in the repo.
- A MaxMind `.mmdb` file is read from `geo/` or `GEOLITE_DB_PATH`.

**Secrets.** `DATABASE_URL`, `APP_SECRET` (falls back to hashing `DATABASE_URL` if unset), `TWO_FACTOR_ENCRYPTION_KEY`, the replica/direct/ClickHouse/Redis/Kafka URLs, and build-time `MAXMIND_LICENSE_KEY` and `GEO_DATABASE_URL`. Roughly 40 non-secret runtime flags are tabulated separately.

**Startup.** `check-db.js` verifies the DB connection and version and runs `prisma migrate deploy` on every boot unless `SKIP_DB_MIGRATION` is set. There is no lock, so multi-replica deployments should gate this. The first migration seeds an `admin`/`umami` user. Then `update-tracker.js` rewrites `public/script.js` if `COLLECT_API_ENDPOINT` is set.

**Scheduled work.** None in the application. The only periodic behaviour is browser-side version polling of api.umami.is.

**Not determinable from code**, called out in the file: whether the `geo/` directory makes it into the Docker image (the Dockerfile never copies it and there is no file-tracing include), who consumes the Kafka topic, who applies the ClickHouse schema, and any sizing or pool configuration.