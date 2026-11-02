output "server_id" {
  value = hcloud_server.app.id
}

output "public_ipv4" {
  value = hcloud_primary_ip.v4.ip_address
}

output "private_ipv4" {
  description = "Address on the private network; the load balancer targets this."
  value       = one(hcloud_server.app.network[*].ip)
}

output "http_ports" {
  description = "Map of http service name to its container port, for load balancer services."
  value       = { for name, s in local.http_services : name => s.port }
}

output "service_names" {
  value = [for s in var.services : s.name]
}
