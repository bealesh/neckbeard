Written to `PRODUCTION_NEEDS.md` in the app directory. The facts, each backed by a file and line in the document:

- **Processes:** one Go binary with three subcommands. `bellwether serve` is the HTTP service, `bellwether work` is a DB-polling worker, and `bellwether report` is a run-once job that prints a summary and exits. The image defaults to `serve`.
- **Port and health:** listens on `$PORT`, default 8080, plain HTTP. Health check is `GET /healthz`. It returns 503 during the optional startup delay and whenever the database ping fails, so a DB outage pulls the service from rotation.
- **Datastore:** PostgreSQL only, found through `DATABASE_URL`. If unset, every process silently falls back to private in-memory storage, so serve, work, and report would not share data. Production must set it for all three.
- **Env vars:** `DATABASE_URL` is the only secret. `PORT`, `APP_ENV`, and `BELLWETHER_STARTUP_DELAY` are plain config. Nothing validates `APP_ENV`.
- **Startup:** all three processes ping the DB with a 10 second timeout and run `CREATE TABLE IF NOT EXISTS notes` inline. That is the only migration mechanism, it needs `CREATE` privilege, and failure is fatal with no retry.
- **Schedules:** the worker ticks every 5 seconds. The schedule for `report` is not in the code and must come from outside.

One thing you should know before deploying: the committed Dockerfile is broken. Line 12 has a `COPY` with no destination and line 14 has an invalid `ENTRYPOINT`, so the image neither builds nor starts as written. The document records the intent I could infer and what I could not confirm.