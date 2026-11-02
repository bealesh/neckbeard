# Bootstrap for one Hetzner environment: the state bucket in Object Storage
# that everything else uses. Unlike the other clouds there is no identity to
# federate — Hetzner has no OIDC trust — so the CI credentials this root tells
# you to set are the project API token and an Object Storage key pair, as
# plain secrets. docs/bootstrap.md states what that means.

resource "minio_s3_bucket" "tfstate" {
  bucket = "${var.name_prefix}-tfstate"
  acl    = "private"
}

resource "minio_s3_bucket_versioning" "tfstate" {
  bucket = minio_s3_bucket.tfstate.bucket
  versioning_configuration {
    status = "Enabled"
  }
}

locals {
  endpoint      = "${var.region}.your-objectstorage.com"
  registry_host = var.vcs == "gitlab" ? "registry.gitlab.com" : "ghcr.io"
}
