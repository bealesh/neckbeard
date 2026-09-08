output "repository_url" {
  value = "${var.region}-docker.pkg.dev/${data.google_project.this.project_id}/${google_artifact_registry_repository.this.repository_id}"
}
