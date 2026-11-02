Written to `PRODUCTION_NEEDS.md` in the repo root. The whole application is three files, so every claim cites `main.go`, `Dockerfile`, or `go.mod` by line.

What it needs, in short:

- **Three processes from one binary:** `serve` is the HTTP service, `work` is a long-running worker polling every 5 seconds, and `report` runs once and exits. The mode is the first command-line argument (`main.go:47-57`).
- **Port and health:** listens on `$PORT`, default 8080, plain HTTP. Health check is `GET /healthz`. It pings PostgreSQL on every call, so it behaves as a readiness check and will pull instances during a database outage. `GET /` works as a database-free liveness probe.
- **One datastore:** PostgreSQL via pgx, located by `DATABASE_URL`. The code falls back to in-memory storage when the variable is blank, which means the worker and the service would not share data. In production it must be set.
- **Config:** four variables are read. Only `DATABASE_URL` is a secret. `PORT` and `APP_ENV` are plain settings, and `BELLWETHER_STARTUP_DELAY` is a test knob that should stay unset.
- **Startup:** every mode pings the database with a 10-second timeout and exits if that fails, then runs an inline `CREATE TABLE IF NOT EXISTS notes`. That is the only migration, so the database role needs CREATE TABLE privilege.
- **Schedule:** the worker's 5-second tick is the only schedule in the code. No cadence for `report` exists anywhere in the repo, and the document says so.

One finding you should know about before deploying: the Dockerfile as committed does not build. Lines 9, 12, and 14 are mangled, so the build output path, the COPY into the runtime image, and the ENTRYPOINT are all broken. I confirmed the build flag error with the local Go 1.27.1 toolchain and confirmed the Go source itself compiles and passes `go vet`. The document records this rather than guessing the intended binary path.