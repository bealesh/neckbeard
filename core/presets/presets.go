// Package presets embeds the tier presets: explicit dimensions and the module
// inputs / usage numbers each tier expands to (DESIGN §4.2).
package presets

import (
	"bytes"
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed presets.yaml
var presetsYAML []byte

func YAML() []byte { return bytes.Clone(presetsYAML) }

type Set struct {
	Version        int                                  `yaml:"version"`
	Tiers          map[string]Tier                      `yaml:"tiers"`
	EnvAdjustments map[string]map[string]map[string]any `yaml:"env_adjustments"`
}

type Tier struct {
	Dimensions Dimensions                `yaml:"dimensions"`
	Usage      map[string]float64        `yaml:"usage"`
	Modules    map[string]map[string]any `yaml:"modules"`
}

type Dimensions struct {
	TrafficBand           string  `yaml:"traffic_band"`
	AvailabilityObjective string  `yaml:"availability_objective"`
	RPOHours              float64 `yaml:"rpo_hours"`
	RTOHours              float64 `yaml:"rto_hours"`
	OpsCapacity           string  `yaml:"ops_capacity"`
}

// Load returns the embedded preset set.
func Load() (*Set, error) {
	dec := yaml.NewDecoder(bytes.NewReader(presetsYAML))
	dec.KnownFields(true)
	var s Set
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decoding embedded presets: %w", err)
	}
	if len(s.Tiers) == 0 {
		return nil, fmt.Errorf("embedded presets contain no tiers")
	}
	return &s, nil
}
