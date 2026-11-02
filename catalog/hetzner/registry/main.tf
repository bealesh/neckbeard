# Hetzner has no container registry. Images live in the VCS's registry, which
# the generated pipelines already authenticate to for pushes; the server pulls
# with a read-only deploy token the operator writes into /etc/neckbeard/registry.env.

locals {
  registry_host = var.vcs == "gitlab" ? "registry.gitlab.com" : "ghcr.io"
  repository    = "${local.registry_host}/${lower(var.repo)}"
}
