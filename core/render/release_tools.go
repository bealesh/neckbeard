package render

import (
	"fmt"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
)

// Every download in the generated install script is integrity-pinned: the job
// that runs it later holds deployment credentials, so a version pin alone still
// trusts the transport and the origin's future contents. Checksums come from
// the vendor's published checksum files where they exist; AWS and Google
// publish none for these archives, so those were recorded from a download
// verified by hand (archive integrity + contents) and dated. A version bump
// without its checksum fails rendering rather than shipping an unverified URL.
var opentofuSHA256 = map[string]string{
	// tofu_<version>_SHA256SUMS on the matching GitHub release.
	"1.12.6": "5dc43da4f750f33873dc25e94587128709e819e544b7be9016b255316153c3a8",
}

const (
	awscliVersion = "2.34.55"
	awscliSHA256  = "dabc146bb8fb40e09f5eb7163fb676f6f6bd4e8c59b1b36de4d8041d2e60b195" // recorded 2026-09-11; AWS publishes only PGP signatures for this archive

	gcloudVersion = "571.0.0"
	gcloudSHA256  = "e537598d1c7b6839ddce95ce25dbaf25571322886c1f3464dd74771e568fcfdd" // recorded 2026-09-11; no published .sha256 sibling

	kubectlVersion = "v1.35.8"
	kubectlSHA256  = "874d5e72dbb819f43cff16bcd1e4f8bac5b7f2361fe1e55049b0a6c676fb0cbf" // published: dl.k8s.io .../kubectl.sha256

	fluxVersion = "2.9.5"
	fluxSHA256  = "b853df82adfd7736f580692f9f734473d571606307139f8fd20c2a80dd1ff473" // published: flux_<version>_checksums.txt

	kubeloginVersion = "v0.2.19"
	kubeloginSHA256  = "ebaeff02aa899c5cae6a2b954b64fc02738185319df2570f7dc053451efa4b2f" // published: .zip.sha256 on the release
)

// Shared-runner setup uses published versions, not an unpublished neckbeard
// container. This script is run in the generated Debian-based GitLab job.
// azure-cli is the one install without our own hash pin: pip verifies the
// artifact against PyPI's metadata for the pinned version, and hash-pinning
// would require locking its full transitive dependency set.
func releaseTools(bp *blueprint.Blueprint) []byte {
	tofuSHA, ok := opentofuSHA256[bp.Pins.OpenTofu]
	if !ok {
		panic(fmt.Sprintf("no pinned sha256 for OpenTofu %s: add it to opentofuSHA256 in core/render/release_tools.go from the release's SHA256SUMS", bp.Pins.OpenTofu))
	}
	script := `#!/usr/bin/env bash
set -euo pipefail
apt-get update -qq
apt-get install -y --no-install-recommends ca-certificates curl unzip python3 python3-venv git skopeo >/dev/null
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
# fetch <url> <dest> <sha256>: a download that does not match its recorded
# checksum fails the job before anything is installed or executed.
fetch() {
  curl -fsSL "$1" -o "$2"
  echo "$3  $2" | sha256sum -c - >/dev/null
}
fetch "https://github.com/opentofu/opentofu/releases/download/vTOFU_VERSION/tofu_TOFU_VERSION_linux_amd64.zip" "$work/tofu.zip" "TOFU_SHA256"
unzip -q "$work/tofu.zip" -d "$work/tofu"
install "$work/tofu/tofu" /usr/local/bin/tofu
`
	switch bp.Cloud {
	case "aws":
		script += `fetch "https://awscli.amazonaws.com/awscli-exe-linux-x86_64-AWSCLI_VERSION.zip" "$work/aws.zip" "AWSCLI_SHA256"
unzip -q "$work/aws.zip" -d "$work"
"$work/aws/install" --update >/dev/null
`
	case "gcp":
		script += `fetch "https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-GCLOUD_VERSION-linux-x86_64.tar.gz" "$work/gcloud.tar.gz" "GCLOUD_SHA256"
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
		script += `fetch "https://dl.k8s.io/release/KUBECTL_VERSION/bin/linux/amd64/kubectl" "$work/kubectl" "KUBECTL_SHA256"
install "$work/kubectl" /usr/local/bin/kubectl
fetch "https://github.com/fluxcd/flux2/releases/download/vFLUX_VERSION/flux_FLUX_VERSION_linux_amd64.tar.gz" "$work/flux.tar.gz" "FLUX_SHA256"
tar -xzf "$work/flux.tar.gz" -C "$work"
install "$work/flux" /usr/local/bin/flux
`
		if bp.Cloud == "azure" {
			script += `fetch "https://github.com/Azure/kubelogin/releases/download/KUBELOGIN_VERSION/kubelogin-linux-amd64.zip" "$work/kubelogin.zip" "KUBELOGIN_SHA256"
unzip -q "$work/kubelogin.zip" -d "$work/kubelogin"
install "$work/kubelogin/bin/linux_amd64/kubelogin" /usr/local/bin/kubelogin
`
		}
	}
	return []byte(strings.NewReplacer(
		"TOFU_VERSION", bp.Pins.OpenTofu,
		"TOFU_SHA256", tofuSHA,
		"AWSCLI_VERSION", awscliVersion,
		"AWSCLI_SHA256", awscliSHA256,
		"GCLOUD_VERSION", gcloudVersion,
		"GCLOUD_SHA256", gcloudSHA256,
		"KUBECTL_VERSION", kubectlVersion,
		"KUBECTL_SHA256", kubectlSHA256,
		"FLUX_VERSION", fluxVersion,
		"FLUX_SHA256", fluxSHA256,
		"KUBELOGIN_VERSION", kubeloginVersion,
		"KUBELOGIN_SHA256", kubeloginSHA256,
	).Replace(script))
}
