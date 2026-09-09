package render

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/catalog"
	"github.com/bealesh/neckbeard/core/config"
	"github.com/bealesh/neckbeard/core/ownership"
	"github.com/bealesh/neckbeard/core/planner"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/core/profile"
)

func testBlueprint(t *testing.T) *blueprint.Blueprint {
	t.Helper()
	base := filepath.Join("..", "planner", "testdata", "basic")
	cfg, cfgDigest, err := config.Load(filepath.Join(base, "neckbeard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	prof, profDigest, err := profile.Load(filepath.Join(base, "app-profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(filepath.Join("..", "..", "catalog", "index.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pre, err := presets.Load()
	if err != nil {
		t.Fatal(err)
	}
	bp, err := planner.Plan(planner.Inputs{
		Config: cfg, ConfigDigest: cfgDigest,
		Profile: prof, ProfileDigest: profDigest,
		Catalog: cat, Presets: pre, PlannerVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bp.Finalize(); err != nil {
		t.Fatal(err)
	}
	return bp
}

var testOpts = Options{CatalogSource: "../../../../catalog-src"}

func TestWriteSetIsDeterministic(t *testing.T) {
	bp := testBlueprint(t)
	a, errA := WriteSet(bp, testOpts)
	b, errB := WriteSet(bp, testOpts)
	if errA != nil || errB != nil {
		t.Fatal(errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Path != b[i].Path || !bytes.Equal(a[i].Content, b[i].Content) || a[i].Owner != b[i].Owner {
			t.Errorf("write-set entry %d differs between renders", i)
		}
	}
}

func TestWriteSetShape(t *testing.T) {
	bp := testBlueprint(t)
	ws, err := WriteSet(bp, testOpts)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]ownership.File{}
	for _, f := range ws {
		byPath[f.Path] = f
	}
	topo, ok := byPath["docs/topology.md"]
	if !ok || topo.Owner != ownership.OwnerGenerated {
		t.Fatal("expected generated docs/topology.md")
	}
	if !bytes.Contains(topo.Content, []byte(bp.Hash)) {
		t.Error("topology doc should carry the blueprint hash")
	}
	if !bytes.Contains(topo.Content, []byte("max_instances=15 (override)")) {
		t.Error("topology doc should mark overridden inputs")
	}
	for _, env := range []string{"dev", "stg", "prd"} {
		f, ok := byPath["infra/envs/"+env+"/custom.tf"]
		if !ok || f.Owner != ownership.OwnerUser {
			t.Errorf("expected user-owned custom.tf for %s", env)
		}
		for _, gen := range []string{"backend.tf", "providers.tf", "main.tf", "outputs.tf"} {
			f, ok := byPath["infra/envs/"+env+"/"+gen]
			if !ok || f.Owner != ownership.OwnerGenerated {
				t.Errorf("expected generated %s for %s", gen, env)
			}
		}
	}

	// Collapse alignment padding so assertions aren't whitespace-brittle.
	main := strings.Join(strings.Fields(string(byPath["infra/envs/dev/main.tf"].Content)), " ")
	for _, want := range []string{
		`module "network" {`,
		`module "runtime_serverless" {`,
		`source = "../../../../catalog-src/catalog/aws/network"`,
		`services = local.services`,
		`allowed_security_group_ids = [module.runtime_serverless.service_security_group_id]`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("dev main.tf missing %q", want)
		}
	}
}

func TestGCPLaneRenders(t *testing.T) {
	base := filepath.Join("..", "planner", "testdata", "basic")
	cfg, cfgDigest, err := config.Load(filepath.Join(base, "neckbeard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Cloud, cfg.Region, cfg.Overrides = "gcp", "us-central1", nil
	cfg.Containers = map[string]string{"dev": "p-dev", "stg": "p-stg", "prd": "p-prd"}
	prof, profDigest, err := profile.Load(filepath.Join(base, "app-profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(filepath.Join("..", "..", "catalog", "index.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pre, err := presets.Load()
	if err != nil {
		t.Fatal(err)
	}
	bp, err := planner.Plan(planner.Inputs{
		Config: cfg, ConfigDigest: cfgDigest, Profile: prof, ProfileDigest: profDigest,
		Catalog: cat, Presets: pre, PlannerVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bp.Finalize(); err != nil {
		t.Fatal(err)
	}
	ws, err := WriteSet(bp, testOpts)
	if err != nil {
		t.Fatal(err)
	}
	var providers, main string
	for _, f := range ws {
		switch f.Path {
		case "infra/envs/dev/providers.tf":
			providers = string(f.Content)
		case "infra/envs/dev/main.tf":
			main = strings.Join(strings.Fields(string(f.Content)), " ")
		}
	}
	if !strings.Contains(providers, `project = "p-dev"`) {
		t.Error("gcp providers.tf must set the env's project")
	}
	if !strings.Contains(main, "private_services_connection = module.network.private_services_connection") {
		t.Error("gcp postgres wiring missing private services connection")
	}
}

func TestK8sLaneRendersDeliveryLayer(t *testing.T) {
	bp := testBlueprint(t)
	bp.Runtime = "kubernetes"
	bp.Environments[0].Modules = nil // module set irrelevant here; lane wiring drives infra files
	ws, err := WriteSet(bp, testOpts)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, f := range ws {
		byPath[f.Path] = string(f.Content)
	}
	for _, p := range []string{
		"clusters/base/apps/kustomization.yaml",
		"clusters/base/apps/deployment-web.yaml",
		"clusters/base/apps/ingress-web.yaml",
		"clusters/base/apps/cronjob-nightly-report.yaml",
		"clusters/dev/apps.yaml",
		"clusters/dev/image-automation.yaml",
		"clusters/prd/apps/kustomization.yaml",
	} {
		if _, ok := byPath[p]; !ok {
			t.Errorf("missing delivery file %s", p)
		}
	}
	// Image automation is dev-only: stg/prd move by promotion PR (§11.2).
	for _, env := range []string{"stg", "prd"} {
		if _, ok := byPath["clusters/"+env+"/image-automation.yaml"]; ok {
			t.Errorf("%s must not have image automation", env)
		}
	}
	// Setter markers live in the DEV overlay (the automation's update path), not
	// the base manifests (review finding 2026-09-08).
	if strings.Contains(byPath["clusters/base/apps/deployment-web.yaml"], `$imagepolicy`) {
		t.Error("base manifests must not carry image-policy markers (invisible to the automation's path)")
	}
	if !strings.Contains(byPath["clusters/dev/apps/kustomization.yaml"], `$imagepolicy`) {
		t.Error("the dev overlay images pin needs the setter markers")
	}
	if strings.Contains(byPath["clusters/stg/apps/kustomization.yaml"], `$imagepolicy`) {
		t.Error("stg/prd move by promotion PR — no automation markers")
	}
	if !strings.Contains(byPath["clusters/dev/apps/kustomization.yaml"], "newTag: bootstrap-pending") {
		t.Error("env overlays must pin the placeholder tag for the release flow to own")
	}
	// The ExternalSecret lives in the apps layer (namespace cycle finding).
	if _, ok := byPath["clusters/dev/apps/external-secret.yaml"]; !ok {
		t.Error("apps layer must carry the ExternalSecret")
	}
	if strings.Contains(byPath["clusters/dev/controllers/app-secrets.yaml"], "kind: ExternalSecret") {
		t.Error("controllers layer must hold only the ClusterSecretStore")
	}
}

func TestUnsupportedLaneIsRefusedByName(t *testing.T) {
	bp := testBlueprint(t)
	// All six launch lanes exist now; a synthetic cloud keeps the refusal path
	// covered (the planner refuses unknown clouds first in real flows).
	bp.Cloud, bp.Runtime = "onprem", "kubernetes"
	_, err := WriteSet(bp, testOpts)
	if err == nil {
		t.Fatal("expected refusal for unimplemented lane")
	}
	if !strings.Contains(err.Error(), "onprem/kubernetes") || !strings.Contains(err.Error(), "no files were written") {
		t.Errorf("refusal should name the lane and promise no writes, got: %v", err)
	}
}

func TestGitCatalogSourcePinsRef(t *testing.T) {
	bp := testBlueprint(t)
	ws, err := WriteSet(bp, Options{CatalogSource: "git::https://github.com/bealesh/neckbeard.git"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ws {
		if f.Path == "infra/envs/dev/main.tf" {
			if !strings.Contains(string(f.Content), `git::https://github.com/bealesh/neckbeard.git//catalog/aws/network?ref=catalog-v0.2.0`) {
				t.Error("git module sources must pin ref=catalog-v<version>")
			}
		}
	}
}

func TestBundledCatalogMustMatchBlueprint(t *testing.T) {
	bp := testBlueprint(t)
	if _, err := WriteSet(bp, Options{}); err == nil {
		t.Fatal("accepted unpinned bundled catalog")
	}
	cat, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	bp.Pins.CatalogDigest = cat.Digest
	ws, err := WriteSet(bp, Options{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range ws {
		if f.Path == bundleDir(bp)+"/catalog/aws/runtime-serverless/main.tf" {
			found = true
		}
	}
	if !found {
		t.Fatal("scaffold did not include its runtime module")
	}
	bp.Pins.CatalogDigest = "sha256:" + strings.Repeat("0", 64)
	if _, err := WriteSet(bp, Options{}); err == nil {
		t.Fatal("silently used a different bundled catalog")
	}
}

func TestExplicitCommandStaysLiteral(t *testing.T) {
	if got := hclValue([]string{"sh", "-c", `echo "${PORT}"; echo '%{literal}'`}); got != `["sh", "-c", "echo \"$${PORT}\"; echo '%%{literal}'"]` {
		t.Fatalf("command became an HCL expression: %s", got)
	}
}
