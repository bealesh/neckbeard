package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoRejectsRelativeAndAbsoluteOutputAliases(t *testing.T) {
	prof, err := filepath.Abs("../../bellwether/app-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Chdir(dir)
	out := filepath.Join(dir, "disco.json")
	for _, force := range []bool{false, true} {
		args := []string{"-profile", prof, "-project", "bellwether", "-out", "./disco.json", "-runbook", out}
		if force {
			args = append(args, "-force")
		}
		if err := runDisco(args); err == nil {
			t.Fatalf("relative and absolute aliases accepted with force=%v", force)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("refused run wrote a file")
		}
	}
}

func TestDiscoOutputs(t *testing.T) {
	prof, err := filepath.Abs("../../bellwether/app-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out, rb := filepath.Join(dir, "disco.json"), filepath.Join(dir, "docs", "disco.md")
	args := []string{"-profile", prof, "-project", "bellwether", "-out", out, "-runbook", rb}

	if err := runDisco([]string{"-profile", prof, "-project", "bellwether", "-out", out, "-runbook", filepath.Join(dir, ".", "disco.json")}); err == nil {
		t.Fatal("same path for -out and -runbook accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("refused run wrote a file")
	}

	if err := runDisco(args); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{out, rb} {
		if fi, err := os.Stat(p); err != nil || fi.Size() == 0 || fi.Mode().Perm() != 0o644 {
			t.Fatalf("%s: %v %v", p, fi, err)
		}
	}

	// the runbook exists, disco.json does not: refuse before writing either
	if err := os.Remove(out); err != nil {
		t.Fatal(err)
	}
	if err := runDisco(args); err == nil {
		t.Fatal("rerun without -force accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("refused rerun wrote disco.json")
	}
	if err := runDisco(append(args, "-force")); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*", ".disco-*")); len(left) > 0 {
		t.Fatalf("temp files left: %v", left)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".disco-*")); len(left) > 0 {
		t.Fatalf("temp files left: %v", left)
	}
}
