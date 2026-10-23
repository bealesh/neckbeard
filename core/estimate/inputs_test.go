package estimate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCostInputsInvalidateCeiling(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "infra/envs/dev")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "deployment.auto.tfvars.json")
	if err := os.WriteFile(path, []byte(`{"public_hostname":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := deploymentInputs(root)
	if err != nil {
		t.Fatal(err)
	}
	res := &Result{Scope: "blueprint", InputDigest: inputDigest("bp", files), Envs: []EnvEstimate{{Env: "dev", Costs: map[string]float64{"expected": 10}}}}
	summary := filepath.Join(root, "costs/estimate.json")
	if err := WriteSummary(summary, Summarize(res, "bp", time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := CheckCeiling(summary, "bp", 100, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"public_hostname":"example.com"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckCeiling(summary, "bp", 100, root); err == nil || !strings.Contains(err.Error(), "inputs changed") {
		t.Fatalf("stale approval accepted: %v", err)
	}
}

func TestPartialOrSingleEnvironmentCannotApproveBlueprint(t *testing.T) {
	for _, res := range []*Result{
		{Scope: "dev", Envs: []EnvEstimate{{Env: "dev", Costs: map[string]float64{"expected": 1}}}},
		{Scope: "blueprint", Envs: []EnvEstimate{{Env: "dev", Unknown: []string{"CPU quantity missing"}, Costs: map[string]float64{"expected": 1}}}},
	} {
		path := filepath.Join(t.TempDir(), "estimate.json")
		if err := WriteSummary(path, Summarize(res, "bp", time.Now())); err != nil {
			t.Fatal(err)
		}
		if err := CheckCeiling(path, "bp", 100); err == nil {
			t.Fatal("incomplete approval accepted")
		}
	}
}
