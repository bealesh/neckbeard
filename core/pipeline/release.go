package pipeline

import (
	"fmt"
	"strings"
)

// Release jobs consume committed intent and serialize by environment. They use
// native runtime APIs; infrastructure applies remain in the infra workflow.
func RenderGitHubRelease(m Model) []byte {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("%s", generatedYAMLHeader)
	w("name: neckbeard-release")
	w("on:")
	w("  workflow_dispatch:")
	w("    inputs:")
	w("      environment:")
	w("        description: Environment with a reviewed release record")
	w("        type: choice")
	w("        required: true")
	w("        options: [dev, stg, prd]")
	w("permissions:")
	w("  contents: read")
	w("jobs:")
	for _, env := range m.Envs {
		w("  release-%s:", env)
		w("    if: github.ref == 'refs/heads/%s' && inputs.environment == '%s'", m.DefaultBranch, env)
		w("    runs-on: ubuntu-latest")
		w("    environment: %s", env)
		w("    concurrency:")
		w("      group: neckbeard-%s", env)
		w("      cancel-in-progress: false")
		w("      queue: max")
		w("    permissions:")
		w("      contents: read")
		w("      id-token: write")
		w("    steps:")
		w("      - uses: actions/checkout@v4")
		w("      - uses: actions/setup-go@v5")
		w("        with:")
		w("          go-version: '1.27.1'")
		w("      - run: sudo apt-get update -qq && sudo apt-get install -y skopeo")
		w("      - uses: opentofu/setup-opentofu@v1")
		w("        with:")
		w("          tofu_version: %s", m.TofuVersion)
		w("          tofu_wrapper: false")
		githubFederation(w, m, env, "apply", "success()")
		if m.Cloud == "gcp" {
			w("      - uses: google-github-actions/setup-gcloud@v2")
			if m.Runtime == "kubernetes" {
				w("        with:")
				w("          install_components: gke-gcloud-auth-plugin")
			}
		}
		if m.Cloud == "azure" {
			w("      - run: az extension add --name containerapp --upgrade --yes")
		}
		if m.Runtime == "kubernetes" {
			w("      - uses: azure/setup-kubectl@v4")
			w("        with:")
			w("          version: v1.35.8")
			w("      - uses: fluxcd/flux2/action@v2.9.5")
			if m.Cloud == "azure" {
				w("      - uses: azure/use-kubelogin@v1")
				w("        with:")
				w("          kubelogin-version: v0.2.19")
			}
		}
		w("      - name: Deploy and verify the committed digest")
		w("        run: |")
		w("          tofu -chdir=infra/envs/%s init -input=false -backend-config=backend.hcl", env)
		w("          cd .neckbeard/release")
		w("          GOWORK=off go run . restore-receipts -root ../.. -env %s", env)
		w("          GOWORK=off go run . stage -root ../.. -env %s", env)
		w("          GOWORK=off go run . deploy -root ../.. -env %s", env)
		w("          GOWORK=off go run . save-receipts -root ../.. -env %s", env)
		w("      - name: Retain verified release and rollback receipt")
		w("        uses: actions/upload-artifact@v4")
		w("        with:")
		w("          name: deployment-%s", env)
		w("          include-hidden-files: true")
		w("          retention-days: 90")
		w("          path: |")
		w("            .neckbeard/deploy/%s.receipt.json", env)
		w("            .neckbeard/deploy/%s.previous.json", env)
	}
	return []byte(b.String())
}
