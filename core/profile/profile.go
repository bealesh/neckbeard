// Package profile loads app-profile.yaml — the agent-written, user-reviewed
// application profile. Epistemic categories (facts / inferences / assumptions /
// confirmed) are kept distinct per DESIGN §8. Secrets carry names and reference
// paths only; values must never appear here.
package profile

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/bealesh/neckbeard/schemas"
	"gopkg.in/yaml.v3"
)

type AppProfile struct {
	Version     int           `yaml:"version"`
	Services    []Service     `yaml:"services"`
	Needs       []Need        `yaml:"needs"`
	Secrets     []SecretRef   `yaml:"secrets,omitempty"`
	Facts       []Fact        `yaml:"facts,omitempty"`
	Inferences  []Inference   `yaml:"inferences,omitempty"`
	Assumptions []Assumption  `yaml:"assumptions,omitempty"`
	Confirmed   []Confirmed   `yaml:"confirmed,omitempty"`
	Unsupported []Unsupported `yaml:"unsupported,omitempty"`
}

type Service struct {
	Name       string `yaml:"name"`
	Kind       string `yaml:"kind"` // http | worker | cron
	Port       int    `yaml:"port,omitempty"`
	HealthPath string `yaml:"health_path,omitempty"`
	Schedule   string `yaml:"schedule,omitempty"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
}

type Need struct {
	Capability string     `yaml:"capability"`
	Mode       string     `yaml:"mode"` // provision | reference
	Evidence   []Evidence `yaml:"evidence,omitempty"`
}

type SecretRef struct {
	Name string `yaml:"name"`
	Ref  string `yaml:"ref,omitempty"`
}

type Evidence struct {
	File string `yaml:"file"`
	Line int    `yaml:"line,omitempty"`
}

type Fact struct {
	ID        string     `yaml:"id"`
	Statement string     `yaml:"statement"`
	Evidence  []Evidence `yaml:"evidence"`
}

type Inference struct {
	ID          string   `yaml:"id"`
	Statement   string   `yaml:"statement"`
	Confidence  string   `yaml:"confidence"`
	Reasoning   string   `yaml:"reasoning,omitempty"`
	DerivedFrom []string `yaml:"derived_from,omitempty"`
}

type Assumption struct {
	ID        string `yaml:"id"`
	Statement string `yaml:"statement"`
}

type Confirmed struct {
	ID       string `yaml:"id"`
	Question string `yaml:"question"`
	Answer   string `yaml:"answer"`
}

type Unsupported struct {
	Capability       string     `yaml:"capability"`
	Detected         string     `yaml:"detected"`
	Explanation      string     `yaml:"explanation"`
	NearestSupported string     `yaml:"nearest_supported,omitempty"`
	Evidence         []Evidence `yaml:"evidence,omitempty"`
}

func Load(path string) (*AppProfile, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return Parse(data)
}

func Parse(data []byte) (*AppProfile, string, error) {
	if err := schemas.ValidateYAML("app-profile", data); err != nil {
		return nil, "", err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var p AppProfile
	if err := dec.Decode(&p); err != nil {
		return nil, "", fmt.Errorf("decoding app-profile.yaml: %w", err)
	}
	return &p, fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}
