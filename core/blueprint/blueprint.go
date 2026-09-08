// Package blueprint defines the generated lockfile: a pure, deterministic function
// of profile + config + manifest + catalog (DESIGN §9). Contains only decisions
// makeable before deployment; deploy-time outputs are never blueprint fields.
package blueprint

import (
	"crypto/sha256"
	"fmt"

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
