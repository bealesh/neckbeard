output "lb_ip_address" {
  description = "Public entry point (HTTP, M1)."
  value       = google_compute_global_address.this.address
}
