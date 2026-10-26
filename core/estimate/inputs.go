package estimate

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bealesh/neckbeard/core/ownership"
)

// deploymentInputs reads local Terraform extensions, never state, backend
// configuration, cached providers or cloud credentials. Generated files must
// still match their ownership record; changes belong in custom.tf.
func deploymentInputs(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	if root == "" {
		return files, nil
	}
	var manifest ownership.Manifest
	b, err := os.ReadFile(filepath.Join(root, ".neckbeard/manifest.json"))
	if err == nil {
		if err = json.Unmarshal(b, &manifest); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for path, record := range manifest.Files {
		if record.Owner != ownership.OwnerGenerated || !strings.HasPrefix(path, ".neckbeard/catalog/") || !strings.HasSuffix(path, ".tf") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		if fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != record.SHA256 {
			return nil, fmt.Errorf("generated catalog cost input %s was modified; restore the pinned catalog before estimating", path)
		}
	}
	for _, env := range []string{"dev", "stg", "prd"} {
		dir := filepath.Join("infra", "envs", env)
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "override.tf" || name == "override.tf.json" || strings.HasSuffix(name, "_override.tf") || strings.HasSuffix(name, "_override.tf.json") {
				return nil, fmt.Errorf("cost input %s overrides generated module settings: use neckbeard.yaml overrides and re-plan so runtime pricing matches deployment", filepath.Join(dir, name))
			}
			if !(strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".tf.json") || strings.HasSuffix(name, ".auto.tfvars") || strings.HasSuffix(name, ".auto.tfvars.json") || name == "terraform.tfvars" || name == "terraform.tfvars.json") {
				continue
			}
			path := filepath.ToSlash(filepath.Join(dir, name))
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("cost input %s must be a regular file", path)
			}
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				return nil, err
			}
			if record, ok := manifest.Files[path]; ok && record.Owner == ownership.OwnerGenerated {
				if fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != record.SHA256 {
					return nil, fmt.Errorf("generated cost input %s was modified; move changes to custom.tf and re-scaffold", path)
				}
				continue
			}
			files[path] = data
		}
	}
	return files, nil
}

func inputDigest(blueprintHash string, files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	fmt.Fprintln(h, "neckbeard-cost-model-v3", blueprintHash)
	for _, name := range names {
		fmt.Fprintf(h, "%s\x00%x\n", name, sha256.Sum256(files[name]))
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}
