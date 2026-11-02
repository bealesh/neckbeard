# Hetzner has no managed PostgreSQL. This module provisions what makes the
# on-box database survive the box: a block-storage volume the runtime mounts at
# /mnt/pgdata and runs postgres against, plus the backup schedule the runtime
# honours. Compute, patching, failover and point-in-time recovery do not exist
# here; the tier presets refuse this capability above smallteam for that reason.

resource "hcloud_volume" "data" {
  name              = "${var.name_prefix}-pgdata"
  size              = var.volume_gb
  location          = var.region
  format            = "ext4"
  delete_protection = var.deletion_protection
  labels = {
    "neckbeard-prefix" = var.name_prefix
    "managed-by"       = "neckbeard"
    "role"             = "pgdata"
  }
}
