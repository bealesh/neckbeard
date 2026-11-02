output "load_balancer_ipv4" {
  value = hcloud_load_balancer.this.ipv4
}

output "load_balancer_id" {
  value = hcloud_load_balancer.this.id
}

output "nameservers" {
  description = "Delegate the zone to these at your registrar or parent zone (empty until public_hostname is set)."
  value       = try(hcloud_zone.this[0].authoritative_nameservers, [])
}

output "https_url" {
  value = local.tls ? "https://${var.public_hostname}" : null
}
