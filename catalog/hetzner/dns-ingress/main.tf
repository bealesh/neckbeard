# HTTPS front door: a managed load balancer terminating TLS with a Hetzner
# managed certificate, forwarding to the application server over the private
# network. DNS lives in Hetzner DNS (hcloud_zone), which is also what managed
# certificates need for validation. Without public_hostname the load balancer
# still exists (HTTP only, by IP) so the lane can be validated and bootstrapped
# before DNS is delegated, the same as the other lanes.

locals {
  labels = {
    "neckbeard-prefix" = var.name_prefix
    "managed-by"       = "neckbeard"
  }
  tls   = var.public_hostname != null && var.public_hostname != ""
  first = var.http_services[0]
}

resource "hcloud_load_balancer" "this" {
  name               = var.name_prefix
  load_balancer_type = var.load_balancer_type
  location           = var.region
  labels             = local.labels
}

resource "hcloud_load_balancer_network" "this" {
  load_balancer_id = hcloud_load_balancer.this.id
  network_id       = var.network_id
}

resource "hcloud_load_balancer_target" "app" {
  type             = "server"
  load_balancer_id = hcloud_load_balancer.this.id
  server_id        = var.server_id
  use_private_ip   = true
  depends_on       = [hcloud_load_balancer_network.this]
}

# The planner sorts http services; the first is the default route. Hetzner load
# balancers route by port, not path, so additional http services get their own
# listen port (443 + index) — an honest limitation of this lane, documented in
# the topology.
resource "hcloud_load_balancer_service" "https" {
  for_each         = local.tls ? { for i, s in var.http_services : s.name => merge(s, { listen = 443 + i }) } : {}
  load_balancer_id = hcloud_load_balancer.this.id
  protocol         = "https"
  listen_port      = each.value.listen
  destination_port = each.value.port

  http {
    certificates  = [hcloud_managed_certificate.this[0].id]
    redirect_http = each.value.listen == 443
  }

  health_check {
    protocol = "http"
    port     = each.value.port
    interval = 15
    timeout  = 10
    retries  = 3
    http {
      path         = each.value.health_path
      status_codes = ["2??", "3??"]
    }
  }
}

resource "hcloud_load_balancer_service" "http" {
  for_each         = local.tls ? {} : { for i, s in var.http_services : s.name => merge(s, { listen = 80 + i }) }
  load_balancer_id = hcloud_load_balancer.this.id
  protocol         = "http"
  listen_port      = each.value.listen
  destination_port = each.value.port

  health_check {
    protocol = "http"
    port     = each.value.port
    interval = 15
    timeout  = 10
    retries  = 3
    http {
      path         = each.value.health_path
      status_codes = ["2??", "3??"]
    }
  }
}

resource "hcloud_managed_certificate" "this" {
  count        = local.tls ? 1 : 0
  name         = "${var.name_prefix}-cert"
  domain_names = [var.public_hostname]
  labels       = local.labels
  depends_on   = [hcloud_zone_rrset.a]
}

resource "hcloud_zone" "this" {
  count  = local.tls && var.manage_zone ? 1 : 0
  name   = var.zone_name
  mode   = "primary"
  ttl    = 300
  labels = local.labels
}

resource "hcloud_zone_rrset" "a" {
  count   = local.tls ? 1 : 0
  zone    = var.manage_zone ? hcloud_zone.this[0].name : var.zone_name
  name    = trimsuffix(trimsuffix(var.public_hostname, var.zone_name), ".")
  type    = "A"
  ttl     = 300
  records = [{ value = hcloud_load_balancer.this.ipv4 }]
}
