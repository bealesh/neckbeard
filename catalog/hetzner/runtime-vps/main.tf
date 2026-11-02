# One server per environment running the application's processes as containers
# under docker compose, brought up by cloud-init and kept up by systemd. HTTP
# services are reached only from the load balancer over the private network;
# workers and cron jobs run on the same box. Postgres, when provisioned, is a
# container on the same box with its data on a block-storage volume.
#
# What is deliberately NOT here: autoscaling, a second box, zero-downtime
# deploys. This lane is for the solo and smallteam tiers; the planner refuses
# the rest with a named reason rather than pretending.

locals {
  http_services   = { for s in var.services : s.name => s if s.kind == "http" }
  worker_services = { for s in var.services : s.name => s if s.kind == "worker" }
  cron_services   = { for s in var.services : s.name => s if s.kind == "cron" }
  postgres        = var.data_volume_id != null
  labels = {
    "neckbeard-prefix" = var.name_prefix
    "managed-by"       = "neckbeard"
  }
  server_type = {
    small  = var.arch == "arm" ? "cax11" : "cx23"
    medium = var.arch == "arm" ? "cax21" : "cx33"
    large  = var.arch == "arm" ? "cax31" : "cx43"
  }[var.server_shape]

  compose = templatefile("${path.module}/templates/compose.yaml.tftpl", {
    image           = var.image
    environment     = var.environment
    http_services   = local.http_services
    worker_services = local.worker_services
    postgres        = local.postgres
    secret_names    = var.secret_names
  })

  cloud_init = templatefile("${path.module}/templates/cloud-init.yaml.tftpl", {
    compose               = local.compose
    secret_names          = var.secret_names
    cron_services         = local.cron_services
    postgres              = local.postgres
    backup_bucket         = var.backup_bucket_name
    backup_endpoint       = var.object_storage_endpoint
    backup_region         = var.region
    environment           = var.environment
    backup_retention_days = var.backup_retention_days
    registry_host         = var.registry_host
    image                 = var.image
  })
}

resource "hcloud_primary_ip" "v4" {
  name        = "${var.name_prefix}-v4"
  type        = "ipv4"
  location    = var.region
  auto_delete = false
  labels      = local.labels
}

resource "hcloud_server" "app" {
  name        = "${var.name_prefix}-app"
  server_type = local.server_type
  image       = "ubuntu-24.04"
  location    = var.region
  user_data   = local.cloud_init
  labels      = local.labels
  ssh_keys    = var.ssh_key_ids
  # A rebuild wipes the box, which is how a bad cloud-init change is fixed, but
  # it must be a deliberate act once anything real runs here.
  rebuild_protection = var.deletion_protection
  delete_protection  = var.deletion_protection

  public_net {
    ipv4_enabled = true
    ipv4         = hcloud_primary_ip.v4.id
    ipv6_enabled = true
  }

  network {
    network_id = var.network_id
  }

  lifecycle {
    # The release flow owns the image (digest bumps via the per-env release
    # state, DESIGN §11.2, D3): it rewrites /etc/neckbeard/image.env over SSH
    # and restarts the unit. user_data only matters at first boot, so a changed
    # image in the blueprint must not propose a replacement server.
    ignore_changes = [user_data]
  }
}

resource "hcloud_firewall_attachment" "app" {
  firewall_id = var.server_firewall_id
  server_ids  = [hcloud_server.app.id]
}

resource "hcloud_volume_attachment" "pgdata" {
  count     = local.postgres ? 1 : 0
  volume_id = var.data_volume_id
  server_id = hcloud_server.app.id
  automount = false # cloud-init mounts it by label so the path is stable across reboots
}
