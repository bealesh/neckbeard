# Hetzner has no secrets manager. The honest substitute: the runtime renders an
# env-file template with these NAMES and empty values at /etc/neckbeard/app.env,
# root-only, outside the repository and outside state. The operator fills values
# over SSH (docs/bootstrap.md); the app reads them as environment variables. This
# module exists so the capability has a home and a reviewable list of names.

locals {
  env_template = join("\n", concat([for n in sort(var.secret_names) : "${n}="], [""]))
}
