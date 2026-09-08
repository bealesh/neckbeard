// Package catalog loads the module catalog index: the golden path, pinned.
package catalog

import (
	"bytes"
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

type Index struct {
	Version      string              `yaml:"version"`
	OpenTofu     string              `yaml:"opentofu"`
	Capabilities map[string][]string `yaml:"capabilities"`
	Clouds       map[string]Cloud    `yaml:"clouds"`
}

type Cloud struct {
	Providers []Provider        `yaml:"providers"`
	Modules   map[string]Module `yaml:"modules"`
	// Capabilities overrides the index-level capability map for this cloud, for
	// honest per-cloud differences (e.g. Azure Container Apps provides ingress
	// natively, so http-ingress maps to no extra module there).
	Capabilities map[string][]string `yaml:"capabilities,omitempty"`
}

// CapabilityModules resolves a capability for one cloud: the cloud's override
// wins, otherwise the index-level map applies.
func (idx *Index) CapabilityModules(cloud, capability string) ([]string, bool) {
	if c, ok := idx.Clouds[cloud]; ok {
		if mods, ok := c.Capabilities[capability]; ok {
			return mods, true
		}
	}
	mods, ok := idx.Capabilities[capability]
	return mods, ok
}

type Provider struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type Module struct {
	Version     string   `yaml:"version"`
	Source      string   `yaml:"source"`
	Overridable []string `yaml:"overridable"`
}

func (m Module) AllowsOverride(key string) bool {
	return slices.Contains(m.Overridable, key)
}

func Load(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var idx Index
	if err := dec.Decode(&idx); err != nil {
		return nil, fmt.Errorf("decoding catalog index: %w", err)
	}
	if idx.Version == "" || idx.OpenTofu == "" {
		return nil, fmt.Errorf("catalog index missing version or opentofu pin")
	}
	return &idx, nil
}
