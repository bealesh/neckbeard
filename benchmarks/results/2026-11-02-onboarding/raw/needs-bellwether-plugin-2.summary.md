Written to `PRODUCTION_NEEDS.md` in the app directory. The repository is one Go binary in a single file plus a Dockerfile, so every fact traces to `main.go` or `Dockerfile` with line numbers.

What the application needs, in brief:

- **Three processes from one binary.** The service runs with `bellwether serve`, the worker with `bellwether work`, and the scheduled job with `bellwether report`. All three share the same startup path and configuration.
- **Port and health.** The service listens on `$PORT`, default 8080, and the load balancer should probe `GET /healthz`. That endpoint pings the database on every call, so a database outage pulls every instance from rotation.
- **One datastore.** PostgreSQL, found only through the `DATABASE_URL` environment variable. A blank value silently switches to a per-process in-memory store, so production must set it on all three modes.
- **Env vars.** Four are read: `DATABASE_URL` (the only secret), `PORT`, `APP_ENV`, and a test-only `BELLWETHER_STARTUP_DELAY` that should stay unset.
- **Startup.** Every mode pings the database with a 10 second timeout and runs an idempotent `CREATE TABLE IF NOT EXISTS notes` migration. Any failure is fatal with no retry, so the database must be up first and the DB user needs DDL rights.
- **Periodic work.** The worker polls every 5 seconds on a hard-coded in-process ticker. The report job has no schedule anywhere in the code, so an external scheduler must be chosen.

Two things worth flagging. The committed Dockerfile is malformed on three lines, verified at the byte level, so the binary path and entrypoint inside the image cannot be determined and the image will not build until it is fixed. Also, the `POST /notes` route has no authentication, so the service should not be exposed publicly without something in front of it.