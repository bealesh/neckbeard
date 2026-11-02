The analysis is written to `PRODUCTION_NEEDS.md` in the app directory. Nothing else in the repo was changed, and no infrastructure was generated.

What the code establishes, with every claim cited to a file and line in the document:

- **One process.** The container runs `sh scripts/start-docker.sh`, which does two one-shot steps and then `exec node server.js`, the Next.js standalone server. There are no background workers and no in-process scheduler. The `umami-mcp` binary in `packages/mcp` is a client-side tool, not a deployed process.
- **Port and health.** Listens on 3000 bound to all interfaces. The health path is `GET /api/heartbeat`, which returns a static 200 and touches no datastore. Both compose files use it.
- **Datastores.** PostgreSQL is required via `DATABASE_URL` (minimum 9.4, enforced at boot). ClickHouse, Redis, and Kafka are optional and enabled by `CLICKHOUSE_URL`, `REDIS_URL`, and `KAFKA_URL` plus `KAFKA_BROKER`. A MaxMind GeoLite2 file under `geo/` is read for IP geolocation. All optional connections open lazily on first use.
- **Secrets.** `DATABASE_URL` and `APP_SECRET` are required. If `APP_SECRET` is missing, the code falls back to hashing the database URL as the signing key. `TWO_FACTOR_ENCRYPTION_KEY` gates 2FA. The replica, direct, ClickHouse, Redis, Kafka, and MaxMind variables all carry credentials. The document separates build-time variables baked into the image from runtime flags.
- **Startup.** `prisma migrate deploy` runs on every container start unless `SKIP_DB_MIGRATION` or `SKIP_DB_CHECK` is set, and a failure stops the container. The first migration seeds an `admin` user with password `umami`.
- **Scheduled work.** None exists server-side. The only timer in the codebase is a browser-side countdown on the 2FA login page.

Three things cannot be determined from the code and are flagged as such in the document: whether the `geo/` directory survives into the runtime image, since the Dockerfile does not copy it explicitly; how the ClickHouse schema and any Kafka consumer are provisioned, since the repo only ships a producer and raw SQL files; and how concurrent migrations are coordinated when several replicas boot at once.