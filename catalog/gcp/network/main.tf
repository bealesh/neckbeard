# Private-by-default network. Cloud Run reaches private ranges (Cloud SQL) through
# direct VPC egress into this subnet; its public egress stays on Google's edge, so
# unlike AWS the solo tier needs no NAT or service endpoints for image pull or
# logging — an honest per-cloud difference (DESIGN §3.3).

resource "google_compute_network" "this" {
  name                    = "${var.name_prefix}-net"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "this" {
  name                     = "${var.name_prefix}-app"
  region                   = var.region
  network                  = google_compute_network.this.id
  ip_cidr_range            = "10.0.0.0/20"
  private_ip_google_access = true
}

# Private services access: Cloud SQL private IP lives in a peered service range.
resource "google_compute_global_address" "psa" {
  name          = "${var.name_prefix}-psa"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.this.id
}

resource "google_service_networking_connection" "psa" {
  network                 = google_compute_network.this.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.psa.name]
}

# Cloud NAT for VPC-routed egress. Regional, so single and per-zone are the same
# thing here; the strategy input is honored, not silently reinterpreted.
resource "google_compute_router" "nat" {
  count   = var.nat_strategy == "none" ? 0 : 1
  name    = "${var.name_prefix}-router"
  region  = var.region
  network = google_compute_network.this.id
}

resource "google_compute_router_nat" "this" {
  count                              = var.nat_strategy == "none" ? 0 : 1
  name                               = "${var.name_prefix}-nat"
  router                             = google_compute_router.nat[0].name
  region                             = var.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

# Custom VPCs already imply deny-all-ingress; this codifies it as an explicit,
# auditable rule (lowest priority, so GKE's own higher-priority allows are
# unaffected). No behavioral change — policy checks want it stated, and so do we.
resource "google_compute_firewall" "deny_all_ingress" {
  name      = "${var.name_prefix}-deny-ingress"
  network   = google_compute_network.this.id
  direction = "INGRESS"
  priority  = 65534

  deny {
    protocol = "all"
  }

  source_ranges = ["0.0.0.0/0"]
}
