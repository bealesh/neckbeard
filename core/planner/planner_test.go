package planner

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/catalog"
	"github.com/bealesh/neckbeard/core/config"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/core/profile"
	"github.com/bealesh/neckbeard/schemas"
)

var update = flag.Bool("update", false, "rewrite golden files")

// plannerVersion is pinned in tests so golden files don't churn with releases.
const plannerVersion = "test"

func loadInputs(t *testing.T, dir string) Inputs {
	t.Helper()
	cfg, cfgDigest, err := config.Load(filepath.Join(dir, "neckbeard.yaml"))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	prof, profDigest, err := profile.Load(filepath.Join(dir, "app-profile.yaml"))
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	// The real catalog index: tests double as proof that the shipped catalog parses.
	cat, err := catalog.Load(filepath.Join("..", "..", "catalog", "index.yaml"))
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	pre, err := presets.Load()
	if err != nil {
		t.Fatalf("presets: %v", err)
	}
	return Inputs{
		Config: cfg, ConfigDigest: cfgDigest,
		Profile: prof, ProfileDigest: profDigest,
		Catalog: cat, Presets: pre,
		PlannerVersion: plannerVersion,
	}
}

func TestGoldenBlueprint(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	bp, err := Plan(in)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	got, err := bp.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	goldenPath := filepath.Join("testdata", "basic", "blueprint.golden.yaml")
	if *update {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden (run `make golden-update`?): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("blueprint differs from golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// Golden tests prove rendering determinism only — never correctness (DESIGN §12.1).
func TestDeterminism(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	var runs [][]byte
	for i := 0; i < 3; i++ {
		bp, err := Plan(in)
		if err != nil {
			t.Fatalf("Plan run %d: %v", i, err)
		}
		out, err := bp.Finalize()
		if err != nil {
			t.Fatalf("Finalize run %d: %v", i, err)
		}
		runs = append(runs, out)
	}
	for i := 1; i < len(runs); i++ {
		if !bytes.Equal(runs[0], runs[i]) {
			t.Fatalf("run %d produced different bytes than run 0", i)
		}
	}
}

func TestBlueprintSatisfiesSchema(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	bp, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := bp.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas.ValidateYAML("blueprint", out); err != nil {
		t.Errorf("generated blueprint violates published contract: %v", err)
	}
}

func TestOverrideNotAllowlisted(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	in.Config.Overrides = map[string]map[string]any{
		"postgres": {"engine_version": "17"},
	}
	_, err := Plan(in)
	if err == nil {
		t.Fatal("expected error for non-allowlisted override, got nil")
	}
	if !strings.Contains(err.Error(), "not overridable") {
		t.Errorf("error should name the refusal, got: %v", err)
	}
}

func TestOverrideOnUnusedModule(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	in.Config.Overrides = map[string]map[string]any{
		"runtime-k8s": {"min_nodes": 5}, // plan uses serverless runtime
	}
	_, err := Plan(in)
	if err == nil {
		t.Fatal("expected error for override on unused module, got nil")
	}
	if !strings.Contains(err.Error(), "does not use") {
		t.Errorf("error should say the module is unused, got: %v", err)
	}
}

func TestUnsupportedCapabilityIsNamedError(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	in.Profile.Needs = append(in.Profile.Needs, profile.Need{Capability: "redis", Mode: "provision"})
	_, err := Plan(in)
	if err == nil {
		t.Fatal("expected error for unsupported capability, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"redis"`) || !strings.Contains(msg, "force-fit") {
		t.Errorf("error should name the capability and refuse force-fitting, got: %v", err)
	}
}

func TestReferenceModeProvisionsNothing(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	for i := range in.Profile.Needs {
		if in.Profile.Needs[i].Capability == "postgres" {
			in.Profile.Needs[i].Mode = "reference"
		}
	}
	bp, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range bp.Environments {
		for _, m := range env.Modules {
			if m.Name == "postgres" {
				t.Fatalf("env %s provisions postgres despite reference mode", env.Name)
			}
		}
	}
	found := false
	for _, r := range bp.References {
		if r.Capability == "postgres" && r.SecretName == "postgres-connection" {
			found = true
		}
	}
	if !found {
		t.Error("expected a postgres reference with secret postgres-connection")
	}
	warned := false
	for _, w := range bp.Warnings {
		if strings.Contains(w, "referenced, not provisioned") {
			warned = true
		}
	}
	if !warned {
		t.Error("expected a referenced-not-provisioned warning")
	}
}

func TestNonAWSCloudsRequireContainers(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	in.Config.Cloud = "gcp"
	in.Config.Overrides = nil
	_, err := Plan(in)
	if err == nil || !strings.Contains(err.Error(), "containers") {
		t.Errorf("gcp without containers should be refused with guidance, got: %v", err)
	}
	in.Config.Containers = map[string]string{"dev": "p-dev", "stg": "p-stg", "prd": "p-prd"}
	bp, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	if bp.Environments[0].Container != "p-dev" {
		t.Errorf("blueprint env should carry its container, got %q", bp.Environments[0].Container)
	}
}

func TestAzureCapabilityOverrideDropsIngressModule(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	in.Config.Cloud = "azure"
	in.Config.Overrides = nil
	in.Config.Containers = map[string]string{"dev": "s1", "stg": "s2", "prd": "s3"}
	bp, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range bp.Environments[0].Modules {
		if m.Name == "dns-ingress" {
			t.Error("azure must not plan a dns-ingress module (Container Apps ingress is native)")
		}
	}
}

func TestKubernetesRuntimeSwapsModuleAndWarns(t *testing.T) {
	in := loadInputs(t, filepath.Join("testdata", "basic"))
	in.Config.Runtime = "kubernetes"
	in.Config.Overrides = nil // basic overrides target runtime-serverless
	bp, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	env := bp.Environments[0]
	hasK8s, hasServerless := false, false
	for _, m := range env.Modules {
		switch m.Name {
		case "runtime-k8s":
			hasK8s = true
		case "runtime-serverless":
			hasServerless = true
		}
	}
	if !hasK8s || hasServerless {
		t.Errorf("kubernetes runtime should select runtime-k8s only (k8s=%v serverless=%v)", hasK8s, hasServerless)
	}
	warned := false
	for _, w := range bp.Warnings {
		if strings.Contains(w, "dedicated cluster") {
			warned = true
		}
	}
	if !warned {
		t.Error("expected dev-cluster cost warning for kubernetes runtime")
	}
}
