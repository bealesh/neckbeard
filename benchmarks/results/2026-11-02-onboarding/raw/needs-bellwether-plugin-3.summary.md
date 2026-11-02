Written to `PRODUCTION_NEEDS.md` in the app directory. Everything is cited to a file and line.

**Summary of findings**

- **Processes:** one Go binary with three modes picked by the first argument at `main.go:47-57`. `serve` is the HTTP service, `work` is a background worker polling every 5 seconds, and `report` runs once and exits.
- **Port and health:** `$PORT`, default 8080, at `main.go:133`. Health check is `GET /healthz`, which returns 503 during a simulated startup delay or when the database ping fails, and 200 otherwise.
- **Datastore:** PostgreSQL only, found through `DATABASE_URL` at `main.go:98`. If unset, each process falls back to a private in-memory store, so the worker and report job cannot see the web service's data. Production effectively requires it.
- **Environment:** four variables. `DATABASE_URL` is the only secret. `PORT`, `APP_ENV`, and `BELLWETHER_STARTUP_DELAY` are plain config, and the last one is a test fixture that should be unset in production.
- **Startup:** all three modes ping the database with a 10 second timeout and then run an inline `CREATE TABLE IF NOT EXISTS` at `main.go:111-118`. Failure is fatal with no retry, so expect a crash loop until the database is reachable. There is no separate migration tool.
- **Schedules:** the worker interval is hard-coded at `main.go:226`. The report job has no schedule anywhere in the repo, so its cadence must be chosen by the deployer.

**One problem worth your attention:** the Dockerfile as committed will not build. Lines 9, 12, and 14 are malformed, with a bad build output path, a `COPY` with no paths, and an invalid `ENTRYPOINT`. The binary's path inside the image, and so the exact start command, cannot be determined until those lines are fixed. The document records this rather than guessing.