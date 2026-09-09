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
	"path/filepath"
	"strings"

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
	Name       string   `yaml:"name"`
	Kind       string   `yaml:"kind"` // http | worker | cron
	Port       int      `yaml:"port,omitempty"`
	HealthPath string   `yaml:"health_path,omitempty"`
	Schedule   string   `yaml:"schedule,omitempty"`
	Dockerfile string   `yaml:"dockerfile,omitempty"`
	Command    []string `yaml:"command,omitempty"`
}

type Need struct {
	SecretName string     `yaml:"secret_name,omitempty"`
	Capability string     `yaml:"capability"`
	Mode       string     `yaml:"mode"` // provision | reference | undecided (draft only)
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
	Disposition      string     `yaml:"disposition,omitempty"` // external | not-required
	Resolution       string     `yaml:"resolution,omitempty"`
	SecretName       string     `yaml:"secret_name,omitempty"`
	Capability       string     `yaml:"capability"`
	Detected         string     `yaml:"detected"`
	Explanation      string     `yaml:"explanation"`
	NearestSupported string     `yaml:"nearest_supported,omitempty"`
	Evidence         []Evidence `yaml:"evidence,omitempty"`
}

// WorkloadError is also used during analysis, before any cloud questions.
func WorkloadError(p *AppProfile) error {
	if len(p.Services) == 0 {
		return fmt.Errorf("no containerized service found: add a Dockerfile before configuring infrastructure")
	}
	images, names := map[string]bool{}, map[string]bool{}
	for _, s := range p.Services {
		if names[s.Name] {
			return fmt.Errorf("duplicate service name %q: give each service a unique name", s.Name)
		}
		names[s.Name] = true
		if s.Dockerfile == "" {
			return fmt.Errorf("service %q needs a dockerfile path", s.Name)
		}
		if filepath.IsAbs(s.Dockerfile) || !filepath.IsLocal(s.Dockerfile) || strings.ContainsAny(s.Dockerfile, "\n\r") {
			return fmt.Errorf("service %q dockerfile must be a repository-relative path", s.Name)
		}
		images[filepath.Clean(s.Dockerfile)] = true
	}
	if len(images) > 1 {
		return fmt.Errorf("multi-image apps are not supported yet: found %d distinct Dockerfiles; the supported shape is one image with optional HTTP, worker, and cron processes", len(images))
	}
	return nil
}

// Ready checks facts that must be settled before planning, independent of the
// agent frontend. Confirmation IDs resolve assumptions without erasing evidence.
func Ready(p *AppProfile) error {
	if err := WorkloadError(p); err != nil {
		return err
	}
	answers := map[string]string{}
	for _, c := range p.Confirmed {
		if _, exists := answers[c.ID]; exists {
			return fmt.Errorf("duplicate confirmed id %q", c.ID)
		}
		answers[c.ID] = strings.TrimSpace(c.Answer)
	}
	var issues []string
	for _, n := range p.Needs {
		if n.Mode == "reference" && strings.TrimSpace(n.SecretName) == "" {
			issues = append(issues, fmt.Sprintf("%s: set secret_name to the environment variable the app uses for its existing connection", n.Capability))
		}
		if n.Mode != "provision" && n.Mode != "reference" {
			issues = append(issues, fmt.Sprintf("%s: choose mode: provision or reference (and secret_name for the existing connection)", n.Capability))
		}
	}
	for _, s := range p.Services {
		if s.Kind == "http" && (s.Port < 1 || s.Port > 65535 || !strings.HasPrefix(s.HealthPath, "/")) {
			issues = append(issues, fmt.Sprintf("service %s: set its HTTP port and health_path", s.Name))
		}
		if s.Kind == "cron" && len(strings.Fields(s.Schedule)) != 5 {
			issues = append(issues, fmt.Sprintf("service %s: supply a five-field cron schedule", s.Name))
		}
	}
	for _, a := range p.Assumptions {
		if answers[a.ID] == "" {
			issues = append(issues, fmt.Sprintf("assumption %s: %s; add a confirmed entry with this id and a nonempty answer, and correct the profile fields", a.ID, a.Statement))
		}
	}
	for _, u := range p.Unsupported {
		if (u.Disposition != "external" && u.Disposition != "not-required") || strings.TrimSpace(u.Resolution) == "" || (u.Disposition == "external" && strings.TrimSpace(u.SecretName) == "") {
			issues = append(issues, fmt.Sprintf("unsupported %s: %s; set disposition: external with secret_name and resolution, or not-required with resolution", u.Capability, u.Explanation))
		}
	}
	if len(issues) > 0 {
		return fmt.Errorf("profile is not ready for planning:\n- %s", strings.Join(issues, "\n- "))
	}
	return nil
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
