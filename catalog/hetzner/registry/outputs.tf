output "repository_url" {
  description = "Image repository (no tag). Pipelines push here; the server pulls from here."
  value       = local.repository
}

output "registry_host" {
  value = local.registry_host
}
