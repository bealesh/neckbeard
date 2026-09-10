package render

import (
	"github.com/bealesh/neckbeard/core/blueprint"
	"strings"
)

// Shared-runner setup uses published versions, not an unpublished neckbeard
// container. This script is run in the generated Debian-based GitLab job.
func releaseTools(bp *blueprint.Blueprint) []byte {
	script := `#!/usr/bin/env bash
set -euo pipefail
apt-get update -qq
apt-get install -y --no-install-recommends ca-certificates curl unzip python3 python3-venv git skopeo >/dev/null
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl -fsSL "https://github.com/opentofu/opentofu/releases/download/vTOFU_VERSION/tofu_TOFU_VERSION_linux_amd64.zip" -o "$work/tofu.zip"
unzip -q "$work/tofu.zip" -d "$work/tofu"
install "$work/tofu/tofu" /usr/local/bin/tofu
`
	switch bp.Cloud {
	case "aws":
		script += `curl -fsSL https://awscli.amazonaws.com/awscli-exe-linux-x86_64-2.34.55.zip -o "$work/aws.zip"
unzip -q "$work/aws.zip" -d "$work"
"$work/aws/install" --update >/dev/null
`
	case "gcp":
		script += `curl -fsSL https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-571.0.0-linux-x86_64.tar.gz -o "$work/gcloud.tar.gz"
tar -xzf "$work/gcloud.tar.gz" -C /opt
/opt/google-cloud-sdk/install.sh --quiet --path-update=false --command-completion=false >/dev/null
ln -sf /opt/google-cloud-sdk/bin/gcloud /usr/local/bin/gcloud
/opt/google-cloud-sdk/bin/gcloud components install gke-gcloud-auth-plugin --quiet >/dev/null
ln -sf /opt/google-cloud-sdk/bin/gke-gcloud-auth-plugin /usr/local/bin/gke-gcloud-auth-plugin
`
	case "azure":
		script += `python3 -m venv /opt/neckbeard-azure
/opt/neckbeard-azure/bin/pip install --quiet azure-cli==2.90.0
ln -sf /opt/neckbeard-azure/bin/az /usr/local/bin/az
az extension add --name containerapp --yes >/dev/null
`
	}
	if bp.Runtime == "kubernetes" {
		script += `curl -fsSL https://dl.k8s.io/release/v1.35.8/bin/linux/amd64/kubectl -o /usr/local/bin/kubectl
chmod +x /usr/local/bin/kubectl
curl -fsSL https://github.com/fluxcd/flux2/releases/download/v2.9.5/flux_2.9.5_linux_amd64.tar.gz -o "$work/flux.tar.gz"
tar -xzf "$work/flux.tar.gz" -C "$work"
install "$work/flux" /usr/local/bin/flux
`
		if bp.Cloud == "azure" {
			script += `curl -fsSL https://github.com/Azure/kubelogin/releases/download/v0.2.19/kubelogin-linux-amd64.zip -o "$work/kubelogin.zip"
unzip -q "$work/kubelogin.zip" -d "$work/kubelogin"
install "$work/kubelogin/bin/linux_amd64/kubelogin" /usr/local/bin/kubelogin
`
		}
	}
	return []byte(replaceToolVersion(script, bp.Pins.OpenTofu))
}
func replaceToolVersion(script, version string) string {
	return strings.ReplaceAll(script, "TOFU_VERSION", version)
}
