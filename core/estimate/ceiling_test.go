package estimate

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCeilingFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "estimate.json")

	if err := CheckCeiling(path, "sha256:abc", 0); err != nil {
		t.Fatalf("no ceiling → no check, got %v", err)
	}
	if err := CheckCeiling(path, "sha256:abc", 100); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing estimate under a ceiling must refuse, got %v", err)
	}

	res := &Result{Scope: "blueprint", Currency: "USD", InfracostVersion: "0.10.45", Envs: []EnvEstimate{
		{Env: "dev", Costs: map[string]float64{"expected": 40}},
		{Env: "prd", Costs: map[string]float64{"expected": 90.5}},
	}}
	if err := WriteSummary(path, Summarize(res, "sha256:abc", time.Unix(0, 0))); err != nil {
		t.Fatal(err)
	}
	if err := CheckCeiling(path, "sha256:other", 1000); err == nil || !strings.Contains(err.Error(), "re-run") {
		t.Fatalf("stale estimate must refuse, got %v", err)
	}
	if err := CheckCeiling(path, "sha256:abc", 130.5); err != nil {
		t.Fatalf("130.5 total at a 130.5 ceiling is within, got %v", err)
	}
	err := CheckCeiling(path, "sha256:abc", 100)
	var ce *CeilingError
	if !errors.As(err, &ce) || ce.Expected != 130.5 || !strings.Contains(err.Error(), "by 30.50") {
		t.Fatalf("over the ceiling must say by how much, got %v", err)
	}
}
