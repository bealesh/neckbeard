package render

import (
	"bytes"
	"path/filepath"
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

func TestWriteSetIsDeterministic(t *testing.T) {
	bp := testBlueprint(t)
	a, b := WriteSet(bp), WriteSet(bp)
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
	ws := WriteSet(bp)
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
	}
}
