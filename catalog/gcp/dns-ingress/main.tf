# Public ingress: a global external Application Load Balancer over serverless NEGs.
# M1 scope is HTTP :80; custom domains + managed TLS wire in with the environment
# manifest (M2/M3), matching the AWS lane.

resource "google_compute_region_network_endpoint_group" "service" {
  for_each              = { for s in var.http_services : s.name => s }
  name                  = "${var.name_prefix}-${each.key}"
  region                = var.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = var.service_names[each.key]
  }
}

resource "google_compute_security_policy" "waf" {
  count = var.waf_enabled ? 1 : 0
  name  = "${var.name_prefix}-waf"

  rule {
    action   = "deny(403)"
    priority = 1000
    match {
      expr {
        expression = "evaluatePreconfiguredWaf('sqli-v33-stable')"
      }
    }
    description = "preconfigured SQLi rules"
  }

  rule {
    action   = "deny(403)"
    priority = 1001
    match {
      expr {
        expression = "evaluatePreconfiguredWaf('xss-v33-stable')"
      }
    }
    description = "preconfigured XSS rules"
  }

  rule {
    action   = "deny(403)"
    priority = 1002
    match {
      expr {
        expression = "evaluatePreconfiguredExpr('cve-canary')"
      }
    }
    description = "log4j/JNDI lookup prevention (CVE-2021-44228)"
  }

  # Cloud Armor requires a catch-all default rule.
  rule {
    action   = "allow"
    priority = 2147483647
    match {
      versioned_expr = "SRC_IPS_V1"
      config {
        src_ip_ranges = ["*"]
      }
    }
    description = "default allow"
  }
}

resource "google_compute_backend_service" "service" {
  for_each              = { for s in var.http_services : s.name => s }
  name                  = "${var.name_prefix}-${each.key}"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  protocol              = "HTTP"
  security_policy       = var.waf_enabled ? google_compute_security_policy.waf[0].id : null

  backend {
    group = google_compute_region_network_endpoint_group.service[each.key].id
  }
}

locals {
  default_service = var.http_services[0].name
  extra_services  = [for i, s in var.http_services : s.name if i > 0]
}

resource "google_compute_url_map" "this" {
  name            = "${var.name_prefix}-lb"
  default_service = google_compute_backend_service.service[local.default_service].id

  dynamic "host_rule" {
    for_each = length(local.extra_services) > 0 ? [1] : []
    content {
      hosts        = ["*"]
      path_matcher = "by-path"
    }
  }

  dynamic "path_matcher" {
    for_each = length(local.extra_services) > 0 ? [1] : []
    content {
      name            = "by-path"
      default_service = google_compute_backend_service.service[local.default_service].id

      dynamic "path_rule" {
        for_each = toset(local.extra_services)
        content {
          paths   = ["/${path_rule.value}/*"]
          service = google_compute_backend_service.service[path_rule.value].id
        }
      }
    }
  }
}

resource "google_compute_target_http_proxy" "this" {
  name    = "${var.name_prefix}-http"
  url_map = var.hostname == null ? google_compute_url_map.this.id : google_compute_url_map.https_redirect[0].id
}

resource "google_compute_global_address" "this" {
  name = "${var.name_prefix}-lb"
}

resource "google_compute_global_forwarding_rule" "http" {
  name                  = "${var.name_prefix}-http"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  port_range            = "80"
  target                = google_compute_target_http_proxy.this.id
  ip_address            = google_compute_global_address.this.id
}

resource "google_compute_managed_ssl_certificate" "this" {
  count = var.hostname == null ? 0 : 1
  name  = "${var.name_prefix}-tls"
  managed { domains = [var.hostname] }
}
resource "google_compute_target_https_proxy" "this" {
  count            = var.hostname == null ? 0 : 1
  name             = "${var.name_prefix}-https"
  url_map          = google_compute_url_map.this.id
  ssl_certificates = [google_compute_managed_ssl_certificate.this[0].id]
}
resource "google_compute_global_forwarding_rule" "https" {
  count                 = var.hostname == null ? 0 : 1
  name                  = "${var.name_prefix}-https"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  port_range            = "443"
  target                = google_compute_target_https_proxy.this[0].id
  ip_address            = google_compute_global_address.this.id
}
resource "google_compute_url_map" "https_redirect" {
  count = var.hostname == null ? 0 : 1
  name  = "${var.name_prefix}-https-redirect"
  default_url_redirect {
    https_redirect         = true
    strip_query            = false
    redirect_response_code = "MOVED_PERMANENTLY_DEFAULT"
  }
}
