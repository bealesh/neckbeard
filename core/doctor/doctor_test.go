package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrerequisites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	checks, err := CheckTools("plan", "1.12.6")
	if err != nil || len(checks) != 0 {
		t.Fatalf("planning needs no external tools: %v %v", checks, err)
	}
	checks, err = CheckTools("estimate", "1.12.6")
	if err != nil || checks[0].Status != "MISSING" {
		t.Fatalf("missing estimator: %v %v", checks, err)
	}
	install := func(name, version string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho '"+version+"'\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	install("infracost", "Infracost v2.0.0")
	checks, _ = CheckTools("estimate", "1.12.6")
	if checks[0].Status != "INCOMPATIBLE" {
		t.Fatalf("accepted wrong major: %v", checks)
	}
	install("infracost-0.10", "Infracost v0.10.45")
	checks, _ = CheckTools("estimate", "1.12.6")
	if checks[0].Status != "READY" || checks[0].Version != "0.10.45" {
		t.Fatalf("classic discovery: %v", checks)
	}
	install("tofu", "OpenTofu v1.11.0")
	checks, _ = CheckTools("validate", "1.12.6")
	if checks[0].Status != "INCOMPATIBLE" {
		t.Fatalf("ignored tool pin: %v", checks)
	}
	if _, err := CheckTools("deploy", "1.12.6"); err == nil {
		t.Fatal("accepted unknown stage")
	}
}
