The write-up is at `PRODUCTION_NEEDS.md` in the repo root, untracked and not committed.

**What the app needs, in brief.** This is umami 3.4.1, a Next.js app that runs as one stateless Node process. The container entrypoint runs a database check and migration, optionally rewrites the tracker script, then execs `node server.js` on port 3000. The load balancer health path is `GET /api/heartbeat`, which returns 200 without touching any datastore.

**Datastores.** PostgreSQL is the only required store and is found through `DATABASE_URL`, with optional replica, direct-migration, and schema variants. ClickHouse, Redis, and Kafka are each optional and enabled only by their URL variables. Kafka is producer-only. Nothing in this repo consumes those topics or applies the ClickHouse schema, so both must be handled outside the app.

**Secrets.** `DATABASE_URL`, `APP_SECRET`, and `TWO_FACTOR_ENCRYPTION_KEY`, plus any credentials embedded in the replica, ClickHouse, Redis, and Kafka URLs. If `APP_SECRET` is unset, the code falls back to hashing `DATABASE_URL` as the token-signing secret, which is worth avoiding.

**Startup.** `prisma migrate deploy` runs on every boot unless `SKIP_DB_MIGRATION` is set, so the runtime database role needs DDL rights. The first migration seeds an `admin` user with password `umami`. The `public/` directory must be writable if `COLLECT_API_ENDPOINT` is used.

**Scheduled work.** None. I searched for timers, cron libraries, workers, and platform cron config. Every hit is browser code, a test, or a build script.

**Two things the code does not settle.** Whether the GeoLite2 database downloaded at build time actually lands inside the Docker image depends on Next.js output tracing, which I could not verify from the repository. And the ClickHouse write path assumes an external Kafka-to-ClickHouse consumer that is not in this codebase. Both are flagged in the document.