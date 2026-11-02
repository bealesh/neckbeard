output "bucket_name" {
  value = minio_s3_bucket.data.bucket
}

output "backup_bucket_name" {
  description = "Bucket the runtime writes nightly database dumps to."
  value       = minio_s3_bucket.backups.bucket
}

output "endpoint" {
  value = "${var.region}.your-objectstorage.com"
}
