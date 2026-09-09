// Package catalogassets distributes the tested catalog with the CLI binary.
package catalogassets

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
)

// Files includes modules, their policies, and the index. No network or source
// checkout is needed to plan, render, or inspect the default catalog.
//
//go:embed index.yaml aws gcp azure
var Files embed.FS

// Digest pins the actual module bytes, not just a mutable version label.
func Digest() string {
	h := sha256.New()
	_ = fs.WalkDir(Files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			data, err := Files.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%s\x00%d\x00", path, len(data))
			h.Write(data)
		}
		return nil
	})
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}
