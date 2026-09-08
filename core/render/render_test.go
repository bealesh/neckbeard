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

func TestUnsupportedLaneIsRefusedByName(t *testing.T) {
	bp := testBlueprint(t)
	bp.Runtime = "kubernetes"
	_, err := WriteSet(bp, testOpts)
	if err == nil {
		t.Fatal("expected refusal for unimplemented lane")
	}
	if !strings.Contains(err.Error(), "aws/kubernetes") || !strings.Contains(err.Error(), "no files were written") {
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
			if !strings.Contains(string(f.Content), `git::https://github.com/bealesh/neckbeard.git//catalog/aws/network?ref=catalog-v0.1.0`) {
				t.Error("git module sources must pin ref=catalog-v<version>")
			}
		}
	}
}
