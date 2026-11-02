# One private network per environment. Servers and the load balancer talk over
# it; the public interface only carries operator SSH (if allowed) and the load
# balancer's own frontend. Hetzner firewalls apply to the public interface, so
# nothing on the private network needs a rule here.

resource "hcloud_network" "this" {
  name     = var.name_prefix
  ip_range = var.ip_range
  labels   = local.labels
}

resource "hcloud_network_subnet" "app" {
  network_id   = hcloud_network.this.id
  type         = "cloud"
  network_zone = var.network_zone
  ip_range     = var.ip_range
}

# Public-interface firewall for application servers. Inbound: operator SSH from
# the listed CIDRs only (empty list = no SSH at all, the honest default), and
# nothing else: HTTP reaches the server from the load balancer over the private
# network. Outbound is open so apt, image pulls and backups work.
resource "hcloud_firewall" "server" {
  name   = "${var.name_prefix}-server"
  labels = local.labels

  dynamic "rule" {
    for_each = length(var.ssh_allowed_cidrs) > 0 ? [1] : []
    content {
      description = "operator ssh"
      direction   = "in"
      protocol    = "tcp"
      port        = "22"
      source_ips  = var.ssh_allowed_cidrs
    }
  }

  rule {
    description = "icmp"
    direction   = "in"
    protocol    = "icmp"
    source_ips  = ["0.0.0.0/0", "::/0"]
  }
}

locals {
  labels = {
    "neckbeard-prefix" = var.name_prefix
    "managed-by"       = "neckbeard"
  }
}
