# Container registry. Artifact Registry has no immutable-tags switch (an honest
# difference from ECR, §3.3) — the promotion flow still moves digests, never tags,
# and the release harness verifies digest-pinned deploys (DESIGN §11.2).

data "google_project" "this" {}

resource "google_artifact_registry_repository" "this" {
  location      = var.region
  repository_id = var.name_prefix
  format        = "DOCKER"

  cleanup_policies {
    id     = "expire-untagged"
    action = "DELETE"
    condition {
      tag_state  = "UNTAGGED"
      older_than = "1209600s" # 14 days, matching the ECR lifecycle policy
    }
  }
}
