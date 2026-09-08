# Secret containers only: no versions are created here, so no value ever passes
# through neckbeard or its state (DESIGN §8, §10.1). A Cloud Run service that
# references a secret will not start until an operator adds a version — that is
# the intended fail-closed behavior, not a bug.

resource "google_secret_manager_secret" "this" {
  for_each  = toset(var.secret_names)
  secret_id = "${var.name_prefix}-${each.value}"

  replication {
    auto {}
  }
}
