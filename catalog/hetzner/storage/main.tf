# Hetzner Object Storage is S3-compatible and has no hcloud resource; buckets are
# created through the MinIO provider against the location endpoint. Credentials
# are an operator-created S3 key pair supplied as MINIO_USER / MINIO_PASSWORD to
# the plan/apply identity, never written here (DESIGN §10.1).

resource "minio_s3_bucket" "data" {
  bucket = "${var.name_prefix}-data"
  acl    = "private"
}

resource "minio_s3_bucket_versioning" "data" {
  count  = var.versioning ? 1 : 0
  bucket = minio_s3_bucket.data.bucket
  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket" "backups" {
  bucket = "${var.name_prefix}-backups"
  acl    = "private"
}
