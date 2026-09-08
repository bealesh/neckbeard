package ownership

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func file(path, content string, owner Owner) File {
	return File{Path: path, Content: []byte(content), Owner: owner}
}

func readFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func exists(root, path string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
	return err == nil
}

func TestFreshApplyCreatesFilesAndManifest(t *testing.T) {
	root := t.TempDir()
	res, err := Apply(root, "sha256:aa", []File{
		file("docs/topology.md", "topo v1", OwnerGenerated),
		file("infra/envs/dev/custom.tf", "# yours", OwnerUser),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflicts() {
		t.Fatalf("unexpected conflicts: %+v", res.Conflicts)
	}
	if len(res.Created) != 2 {
		t.Errorf("want 2 created, got %v", res.Created)
	}
	if got := readFile(t, root, "docs/topology.md"); got != "topo v1" {
		t.Errorf("content mismatch: %q", got)
	}
	if !exists(root, ManifestPath) {
		t.Error("manifest not written")
	}
}

func TestReapplyUnmodifiedIsIdempotent(t *testing.T) {
	root := t.TempDir()
	ws := []File{file("docs/topology.md", "topo v1", OwnerGenerated)}
	if _, err := Apply(root, "sha256:aa", ws); err != nil {
		t.Fatal(err)
	}
	res, err := Apply(root, "sha256:aa", ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unchanged) != 1 || len(res.Created)+len(res.Updated) != 0 {
		t.Errorf("expected only unchanged, got %+v", res)
	}
}

func TestUnmodifiedFileIsRegeneratedFreely(t *testing.T) {
	root := t.TempDir()
	if _, err := Apply(root, "sha256:aa", []File{file("docs/topology.md", "topo v1", OwnerGenerated)}); err != nil {
		t.Fatal(err)
	}
	res, err := Apply(root, "sha256:bb", []File{file("docs/topology.md", "topo v2", OwnerGenerated)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 || res.HasConflicts() {
		t.Fatalf("expected clean update, got %+v", res)
	}
	if got := readFile(t, root, "docs/topology.md"); got != "topo v2" {
		t.Errorf("content not updated: %q", got)
	}
}

// The §6.2 refusal path: hand-edited generated files are never overwritten.
func TestHandEditedFileConflicts(t *testing.T) {
	root := t.TempDir()
	if _, err := Apply(root, "sha256:aa", []File{file("docs/topology.md", "topo v1", OwnerGenerated)}); err != nil {
		t.Fatal(err)
	}
	edited := "topo v1 — my precious hand edits"
	if err := os.WriteFile(filepath.Join(root, "docs/topology.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Apply(root, "sha256:bb", []File{file("docs/topology.md", "topo v2", OwnerGenerated)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasConflicts() {
		t.Fatal("expected a conflict for the hand-edited file")
	}
	c := res.Conflicts[0]
	if !strings.Contains(c.Reason, "edited") {
		t.Errorf("reason should say edited, got %q", c.Reason)
	}
	if got := readFile(t, root, "docs/topology.md"); got != edited {
		t.Errorf("hand edit was clobbered: %q", got)
	}
	if got := readFile(t, root, c.NewPath); got != "topo v2" {
		t.Errorf("fresh render missing at %s: %q", c.NewPath, got)
	}
}

func TestConflictResolutionByAcceptingNew(t *testing.T) {
	root := t.TempDir()
	if _, err := Apply(root, "sha256:aa", []File{file("docs/topology.md", "topo v1", OwnerGenerated)}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "docs/topology.md"), []byte("edited"), 0o644)
	if _, err := Apply(root, "sha256:bb", []File{file("docs/topology.md", "topo v2", OwnerGenerated)}); err != nil {
		t.Fatal(err)
	}
	// User resolves by taking the fresh render.
	os.Rename(filepath.Join(root, "docs/topology.md"+NewSuffix), filepath.Join(root, "docs/topology.md"))

	res, err := Apply(root, "sha256:bb", []File{file("docs/topology.md", "topo v2", OwnerGenerated)})
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflicts() {
		t.Fatalf("conflict should be resolved, got %+v", res.Conflicts)
	}
	if exists(root, "docs/topology.md"+NewSuffix) {
		t.Error("leftover conflict artifact should be cleaned up")
	}
}

func TestForeignFileIsNeverOverwritten(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.WriteFile(filepath.Join(root, "docs/topology.md"), []byte("I was here first"), 0o644)

	res, err := Apply(root, "sha256:aa", []File{file("docs/topology.md", "topo v1", OwnerGenerated)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasConflicts() {
		t.Fatal("expected conflict for pre-existing foreign file")
	}
	if !strings.Contains(res.Conflicts[0].Reason, "not created by neckbeard") {
		t.Errorf("reason should say foreign, got %q", res.Conflicts[0].Reason)
	}
	if got := readFile(t, root, "docs/topology.md"); got != "I was here first" {
		t.Errorf("foreign file clobbered: %q", got)
	}
}

func TestIdenticalForeignFileIsAdopted(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.WriteFile(filepath.Join(root, "docs/topology.md"), []byte("topo v1"), 0o644)

	res, err := Apply(root, "sha256:aa", []File{file("docs/topology.md", "topo v1", OwnerGenerated)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Adopted) != 1 || res.HasConflicts() {
		t.Fatalf("expected adoption, got %+v", res)
	}
}

func TestUserFilesAreCreatedOnceAndNeverTouched(t *testing.T) {
	root := t.TempDir()
	ws := []File{file("infra/envs/dev/custom.tf", "# stub", OwnerUser)}
	if _, err := Apply(root, "sha256:aa", ws); err != nil {
		t.Fatal(err)
	}
	custom := "resource \"mine\" {}"
	os.WriteFile(filepath.Join(root, "infra/envs/dev/custom.tf"), []byte(custom), 0o644)

	// Re-apply with a different stub: the user's content must survive, no conflict.
	res, err := Apply(root, "sha256:bb", []File{file("infra/envs/dev/custom.tf", "# new stub", OwnerUser)})
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflicts() || len(res.SkippedUser) != 1 {
		t.Fatalf("user file should be skipped without conflict, got %+v", res)
	}
	if got := readFile(t, root, "infra/envs/dev/custom.tf"); got != custom {
		t.Errorf("user content clobbered: %q", got)
	}
}

func TestStaleGeneratedFilesRemovedOnlyIfUnmodified(t *testing.T) {
	root := t.TempDir()
	if _, err := Apply(root, "sha256:aa", []File{
		file("docs/topology.md", "topo", OwnerGenerated),
		file("docs/old-unmodified.md", "old", OwnerGenerated),
		file("docs/old-edited.md", "old", OwnerGenerated),
	}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "docs/old-edited.md"), []byte("old + edits"), 0o644)

	res, err := Apply(root, "sha256:bb", []File{file("docs/topology.md", "topo", OwnerGenerated)})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(res.RemovedStale, "docs/old-unmodified.md") || exists(root, "docs/old-unmodified.md") {
		t.Errorf("unmodified stale file should be removed, got %+v", res)
	}
	if !slices.Contains(res.KeptStale, "docs/old-edited.md") || !exists(root, "docs/old-edited.md") {
		t.Errorf("modified stale file should be kept, got %+v", res)
	}
}

func TestWriteSetRejectsEscapesAndDuplicates(t *testing.T) {
	root := t.TempDir()
	for _, bad := range [][]File{
		{file("../outside.md", "x", OwnerGenerated)},
		{file("/abs.md", "x", OwnerGenerated)},
		{file(ManifestPath, "x", OwnerGenerated)},
		{file("a.md", "x", OwnerGenerated), file("a.md", "y", OwnerGenerated)},
		{file("a.md"+NewSuffix, "x", OwnerGenerated)},
	} {
		if _, err := Apply(root, "sha256:aa", bad); err == nil {
			t.Errorf("expected rejection for write-set %+v", bad)
		}
	}
}
