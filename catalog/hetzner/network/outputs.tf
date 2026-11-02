output "network_id" {
  value = hcloud_network.this.id
}

output "subnet_id" {
  value = hcloud_network_subnet.app.id
}

output "server_firewall_id" {
  value = hcloud_firewall.server.id
}
