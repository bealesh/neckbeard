Written to `PRODUCTION_NEEDS.md` in the repo root, 204 lines, every fact cited by file and line. Nothing else was changed, and the file is uncommitted.

What the document establishes about Umami 3.4.1:

- **One process only.** A Next.js standalone server started by `scripts/start-docker.sh:14-16`, which runs the DB check, the tracker rewrite, then `exec node server.js`. No worker, no scheduler. The only timers in the tree are browser code.
- **Port and health.** Listens on 3000 at `0.0.0.0` per `Dockerfile:99-102`. Health check is `GET /api/heartbeat`, which returns ok without touching a datastore. Behind a load balancer it needs a client IP header from the list at `src/lib/ip.ts:3-14`.
- **Datastores.** PostgreSQL 9.4+ via `DATABASE_URL` is required even when ClickHouse is on. ClickHouse via `CLICKHOUSE_URL`, Kafka via `KAFKA_URL` plus `KAFKA_BROKER`, and Redis via `REDIS_URL` are all optional, each switching a documented code path. GeoLite2 is a local file fetched only at build time.
- **Secrets.** `DATABASE_URL`, `APP_SECRET`, `TWO_FACTOR_ENCRYPTION_KEY`, the replica and direct DB URLs, `CLICKHOUSE_URL`, `REDIS_URL`, `KAFKA_URL`, and the build-time `MAXMIND_LICENSE_KEY`. One notable trap: without `APP_SECRET` the app signs tokens with a hash of `DATABASE_URL` (`src/lib/crypto.ts:57`).
- **Startup.** The container runs `prisma migrate deploy` on every boot unless `SKIP_DB_MIGRATION` is set, so the DB role needs DDL rights and multi-replica first boots can race. The first migration seeds an `admin` user. If `COLLECT_API_ENDPOINT` is set, startup writes to `public/script.js`, which breaks on a read-only filesystem.
- **Scheduled work.** None in the app. The one cron in the repo is a GitHub Actions stale-issue job.

Three things could not be settled from the code and are flagged in the file: whether the GeoLite2 file survives into the final Docker image, how Kafka topics get consumed into ClickHouse, and how the ClickHouse schema is applied, since no script references `db/clickhouse/`.