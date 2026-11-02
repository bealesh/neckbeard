Done. `PRODUCTION_NEEDS.md` is written in the app directory, 208 lines, every claim cited to a file and line. It is untracked and not committed.

**What the app needs, in brief:**

- **One process.** A single Next.js HTTP server started by `exec node server.js` from the Docker entrypoint. There is no worker, no scheduler, and no cron dependency. Two one-shot scripts run before it: the database check with Prisma migrations, and a tracker-script rewrite.
- **Port and health.** Listens on 3000 at 0.0.0.0, overridable with `PORT` and `HOSTNAME`. Health check is `GET /api/heartbeat`, which returns a static JSON body and touches no datastore.
- **Datastores.** PostgreSQL 9.4+ is required via `DATABASE_URL`. ClickHouse, Kafka, and Redis are optional and each switched on by its own URL variable. A GeoLite2 file is read from disk at request time and is fetched at build time.
- **Secrets.** `DATABASE_URL`, `APP_SECRET`, `TWO_FACTOR_ENCRYPTION_KEY`, plus the ClickHouse, Kafka, Redis, replica, and direct-database URLs when used. `APP_SECRET` falls back to hashing the database URL, so rotating either invalidates all sessions.
- **Startup.** Migrations run on every container start via `prisma migrate deploy`, skippable with `SKIP_DB_MIGRATION`. The tracker rewrite writes into the image filesystem when `COLLECT_API_ENDPOINT` is set.
- **Scheduled work.** None server-side. Salt rotation and cache TTLs are time-based but computed per request.

**Things the code does not determine,** listed in the file's last section: how ClickHouse schema and migrations are applied in production, what consumes the Kafka topics, how concurrent migrations from multiple replicas are coordinated, and resource sizing.