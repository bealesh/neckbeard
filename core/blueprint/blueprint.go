// Package blueprint defines the generated lockfile: a pure, deterministic function
// of profile + config + manifest + catalog (DESIGN §9). Contains only decisions
// makeable before deployment; deploy-time outputs are never blueprint fields.
package blueprint

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Blueprint struct {
	Version       int           `yaml:"version"`
	Hash          string        `yaml:"blueprint_hash"`
	GeneratedFrom GeneratedFrom `yaml:"generated_from"`
	Pins          Pins          `yaml:"pins"`
	App           string        `yaml:"app"`
	Org           string        `yaml:"org"`
	Cloud         string        `yaml:"cloud"`
	Region        string        `yaml:"region"`
	Runtime       string        `yaml:"runtime"`
	VCS           string        `yaml:"vcs"`
	Tier          string        `yaml:"tier"`
	Services      []Service     `yaml:"services"`
	Environments  []Environment `yaml:"environments"`
	References    []Reference   `yaml:"references,omitempty"`
	Warnings      []string      `yaml:"warnings,omitempty"`
}

type GeneratedFrom struct {
	NeckbeardYAML       string `yaml:"neckbeard_yaml"`
	AppProfile          string `yaml:"app_profile"`
	EnvironmentManifest string `yaml:"environment_manifest,omitempty"`
}

type Pins struct {
	Catalog   string     `yaml:"catalog"`
	OpenTofu  string     `yaml:"opentofu"`
	Planner   string     `yaml:"planner"`
	Providers []Provider `yaml:"providers,omitempty"`
}

type Provider struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// Service definitions are pre-deploy-known and flow from the app profile into the
// lockfile so renderers can wire runtimes and ingress. Image digests are NOT here:
// they are deploy-time values owned by the release flow (DESIGN §11.2).
type Service struct {
	Name       string `yaml:"name"`
	Kind       string `yaml:"kind"`
	Port       int    `yaml:"port,omitempty"`
	HealthPath string `yaml:"health_path,omitempty"`
	Schedule   string `yaml:"schedule,omitempty"`
}

type Environment struct {
	Name    string        `yaml:"name"`
	Modules []ModuleUsage `yaml:"modules"`
}

type ModuleUsage struct {
	Name    string  `yaml:"name"`
	Source  string  `yaml:"source"`
	Version string  `yaml:"version"`
	Inputs  []Input `yaml:"inputs"`
}

// Input provenance values (DESIGN §9): preset | derived | override-supported |
// override-warned. The golden-path promise is scoped to preset and supported values.
type Input struct {
	Key        string `yaml:"key"`
	Value      any    `yaml:"value"`
	Provenance string `yaml:"provenance"`
}

type Reference struct {
	Capability string `yaml:"capability"`
	SecretName string `yaml:"secret_name"`
}

// Finalize computes blueprint_hash over the canonical form (hash field empty) and
// returns the final canonical YAML. Same blueprint ⇒ same bytes, enforced by tests.
func (b *Blueprint) Finalize() ([]byte, error) {
	b.Hash = ""
	unhashed, err := yaml.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("marshaling blueprint: %w", err)
	}
	b.Hash = fmt.Sprintf("sha256:%x", sha256.Sum256(unhashed))
	return yaml.Marshal(b)
}

// Load reads a blueprint and verifies its hash. The blueprint is a generated
// lockfile: a hash mismatch means it was hand-edited or corrupted, and the fix is
// re-running `neckbeard plan` (or changing neckbeard.yaml), never editing it.
func Load(path string) (*Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var b Blueprint
	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("decoding blueprint: %w", err)
	}
	stored := b.Hash
	b.Hash = ""
	canonical, err := yaml.Marshal(&b)
	if err != nil {
		return nil, fmt.Errorf("re-canonicalizing blueprint: %w", err)
	}
	computed := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	if stored != computed {
		return nil, fmt.Errorf("blueprint hash mismatch (recorded %s, computed %s): %s is generated and must not be edited by hand — change neckbeard.yaml or app-profile.yaml and re-run `neckbeard plan`", stored, computed, path)
	}
	b.Hash = stored
	return &b, nil
}
