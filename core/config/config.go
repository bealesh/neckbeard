// Package config loads and validates neckbeard.yaml, the user-owned configuration.
package config

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/bealesh/neckbeard/schemas"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Version      int                       `yaml:"version"`
	App          string                    `yaml:"app"`
	Org          string                    `yaml:"org"`
	Cloud        string                    `yaml:"cloud"`
	Region       string                    `yaml:"region"`
	VCS          string                    `yaml:"vcs"`
	Runtime      string                    `yaml:"runtime"`
	Tier         string                    `yaml:"tier"`
	EntryPath    string                    `yaml:"entry_path"`
	Repo         string                    `yaml:"repo"`
	Environments []string                  `yaml:"environments,omitempty"`
	ManifestRef  string                    `yaml:"manifest_ref,omitempty"`
	Containers   map[string]string         `yaml:"containers,omitempty"`
	FinOps       FinOps                    `yaml:"finops,omitempty"`
	Overrides    map[string]map[string]any `yaml:"overrides,omitempty"`
}

type FinOps struct {
	Currency           string             `yaml:"currency,omitempty"`
	MonthlyBudgetAlert float64            `yaml:"monthly_budget_alert,omitempty"`
	Usage              map[string]float64 `yaml:"usage,omitempty"`
}

// EnvironmentOrder is the fixed launch environment set (DESIGN §3.4).
var EnvironmentOrder = []string{"dev", "stg", "prd"}

// Load reads, schema-validates, and strictly decodes neckbeard.yaml.
// The returned digest ("sha256:<hex>") goes into the blueprint's generated_from.
func Load(path string) (*Config, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, string, error) {
	if err := schemas.ValidateYAML("neckbeard", data); err != nil {
		return nil, "", err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, "", fmt.Errorf("decoding neckbeard.yaml: %w", err)
	}
	if len(c.Environments) == 0 {
		c.Environments = append([]string(nil), EnvironmentOrder...)
	}
	return &c, fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}
