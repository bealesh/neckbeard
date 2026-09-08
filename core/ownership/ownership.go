// Package ownership implements the file-ownership zones and regeneration contract
// (DESIGN §6.1–6.2): every generated file is tracked in .neckbeard/manifest.json
// with its content hash; regeneration replaces a file only when it is unmodified.
// A hand-edited or foreign file is never silently overwritten — the fresh render
// lands next to it as <file>.neckbeard-new and the apply reports a conflict.
package ownership

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	ManifestDir  = ".neckbeard"
	ManifestPath = ".neckbeard/manifest.json"
	// NewSuffix marks where a conflicting fresh render is written.
	NewSuffix = ".neckbeard-new"
)

type Owner string

const (
	// OwnerGenerated files are regenerated freely while unmodified.
	OwnerGenerated Owner = "generated"
	// OwnerUser files (extension points) are created once and never touched again.
	OwnerUser Owner = "user"
)

type Manifest struct {
	Version       int                  `json:"version"`
	BlueprintHash string               `json:"blueprint_hash"`
	Files         map[string]FileEntry `json:"files"`
}

type FileEntry struct {
	Owner  Owner  `json:"owner"`
	SHA256 string `json:"sha256"`
}

// File is one entry of a renderer's write-set. Path is slash-separated and
// relative to the apply root. Mode 0 means the default 0644.
type File struct {
	Path    string
	Content []byte
	Owner   Owner
	Mode    os.FileMode
}

type Conflict struct {
	Path    string
	Reason  string
	NewPath string // where the fresh render was written instead
}

type Result struct {
	Created      []string
	Updated      []string
	Unchanged    []string
	Adopted      []string // existing bytes already matched the render; now tracked
	SkippedUser  []string // user-owned files that already exist
	RemovedStale []string // previously generated, no longer rendered, unmodified
	KeptStale    []string // no longer rendered but modified or user-owned; left alone
	Conflicts    []Conflict
}

func (r *Result) HasConflicts() bool { return len(r.Conflicts) > 0 }

// Apply reconciles the write-set against the root directory under the regeneration
// contract and rewrites the manifest to match what is actually on disk afterwards.
// It returns the result and a nil error even when there are conflicts; callers
// decide the exit code.
func Apply(root, blueprintHash string, files []File) (*Result, error) {
	if err := checkWriteSet(files); err != nil {
		return nil, err
	}
	old, err := loadManifest(root)
	if err != nil {
		return nil, err
	}

	res := &Result{}
	next := Manifest{Version: 1, BlueprintHash: blueprintHash, Files: map[string]FileEntry{}}

	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(a, b File) int { return strings.Compare(a.Path, b.Path) })

	for _, f := range sorted {
		full := filepath.Join(root, filepath.FromSlash(f.Path))
		disk, readErr := os.ReadFile(full)
		exists := readErr == nil
		if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
			return nil, fmt.Errorf("reading %s: %w", f.Path, readErr)
		}
		newHash := hashBytes(f.Content)

		if f.Owner == OwnerUser {
			if exists {
				// Theirs now, whatever its content; track it so stale logic knows it.
				res.SkippedUser = append(res.SkippedUser, f.Path)
				next.Files[f.Path] = FileEntry{Owner: OwnerUser, SHA256: hashBytes(disk)}
				continue
			}
			if err := writeFile(full, f.Content, f.Mode); err != nil {
				return nil, err
			}
			res.Created = append(res.Created, f.Path)
			next.Files[f.Path] = FileEntry{Owner: OwnerUser, SHA256: newHash}
			continue
		}

		switch {
		case !exists:
			if err := writeFile(full, f.Content, f.Mode); err != nil {
				return nil, err
			}
			res.Created = append(res.Created, f.Path)
			next.Files[f.Path] = FileEntry{Owner: OwnerGenerated, SHA256: newHash}

		case hashBytes(disk) == newHash:
			// Disk already matches the render; adopt if it wasn't tracked.
			if _, tracked := old.Files[f.Path]; tracked {
				res.Unchanged = append(res.Unchanged, f.Path)
			} else {
				res.Adopted = append(res.Adopted, f.Path)
			}
			next.Files[f.Path] = FileEntry{Owner: OwnerGenerated, SHA256: newHash}

		case old.Files[f.Path].SHA256 == hashBytes(disk):
			// Tracked and unmodified since we wrote it: regenerate freely.
			if err := writeFile(full, f.Content, f.Mode); err != nil {
				return nil, err
			}
			res.Updated = append(res.Updated, f.Path)
			next.Files[f.Path] = FileEntry{Owner: OwnerGenerated, SHA256: newHash}

		default:
			// Hand-edited, or a file neckbeard did not create. Never overwrite.
			reason := "file was edited after generation"
			if _, tracked := old.Files[f.Path]; !tracked {
				reason = "file exists but was not created by neckbeard"
			}
			newPath := f.Path + NewSuffix
			if err := writeFile(filepath.Join(root, filepath.FromSlash(newPath)), f.Content, f.Mode); err != nil {
				return nil, err
			}
			res.Conflicts = append(res.Conflicts, Conflict{Path: f.Path, Reason: reason, NewPath: newPath})
			// Manifest keeps reflecting the last state we were responsible for.
			if entry, tracked := old.Files[f.Path]; tracked {
				next.Files[f.Path] = entry
			}
			continue
		}
		// A successful reconcile clears any leftover conflict artifact.
		_ = os.Remove(full + NewSuffix)
	}

	// Stale entries: tracked before, not in this write-set.
	inSet := map[string]bool{}
	for _, f := range sorted {
		inSet[f.Path] = true
	}
	for _, path := range slices.Sorted(maps.Keys(old.Files)) {
		if inSet[path] {
			continue
		}
		entry := old.Files[path]
		full := filepath.Join(root, filepath.FromSlash(path))
		disk, readErr := os.ReadFile(full)
		if errors.Is(readErr, fs.ErrNotExist) {
			continue // already gone; just drop the entry
		}
		if readErr != nil {
			return nil, fmt.Errorf("reading stale %s: %w", path, readErr)
		}
		if entry.Owner == OwnerGenerated && hashBytes(disk) == entry.SHA256 {
			if err := os.Remove(full); err != nil {
				return nil, fmt.Errorf("removing stale %s: %w", path, err)
			}
			res.RemovedStale = append(res.RemovedStale, path)
		} else {
			// Modified since generation, or user-owned: not ours to delete.
			res.KeptStale = append(res.KeptStale, path)
		}
	}

	if err := saveManifest(root, next); err != nil {
		return nil, err
	}
	return res, nil
}

func checkWriteSet(files []File) error {
	seen := map[string]bool{}
	for _, f := range files {
		p := f.Path
		if p == "" || strings.HasPrefix(p, "/") || filepath.IsAbs(p) {
			return fmt.Errorf("write-set path %q must be relative", p)
		}
		for _, seg := range strings.Split(p, "/") {
			if seg == ".." {
				return fmt.Errorf("write-set path %q escapes the root", p)
			}
		}
		if p == ManifestPath {
			return fmt.Errorf("write-set must not target %s", ManifestPath)
		}
		if strings.HasSuffix(p, NewSuffix) {
			return fmt.Errorf("write-set path %q collides with the conflict suffix", p)
		}
		if f.Owner != OwnerGenerated && f.Owner != OwnerUser {
			return fmt.Errorf("write-set path %q has invalid owner %q", p, f.Owner)
		}
		if seen[p] {
			return fmt.Errorf("write-set contains %q twice", p)
		}
		seen[p] = true
	}
	return nil
}

func loadManifest(root string) (Manifest, error) {
	m := Manifest{Version: 1, Files: map[string]FileEntry{}}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ManifestPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parsing %s: %w", ManifestPath, err)
	}
	if m.Files == nil {
		m.Files = map[string]FileEntry{}
	}
	return m, nil
}

func saveManifest(root string, m Manifest) error {
	// encoding/json sorts map keys: the manifest is deterministic by construction.
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(root, filepath.FromSlash(ManifestPath)), append(data, '\n'), 0)
}

func writeFile(full string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	return os.WriteFile(full, content, mode)
}

func hashBytes(b []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(b))
}
