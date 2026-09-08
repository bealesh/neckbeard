# Application object storage: uniform access, public access prevented
# (DESIGN §10.1 — org policies additionally deny public buckets on Path F).

resource "google_storage_bucket" "data" {
  name                        = "${var.name_prefix}-data"
  location                    = var.region
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = var.versioning
  }
}
