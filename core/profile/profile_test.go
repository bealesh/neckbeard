package profile

import (
	"strings"
	"testing"
)

func TestReadinessGates(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*AppProfile)
	}{
		{"image defaults", "", func(p *AppProfile) {}},
		{"no image", "dockerfile", func(p *AppProfile) { p.Services[0].Dockerfile = "" }},
		{"outside repo", "repository-relative", func(p *AppProfile) { p.Services[0].Dockerfile = "../Dockerfile" }},
		{"second image", "multi-image", func(p *AppProfile) {
			p.Services = append(p.Services, Service{Name: "worker", Kind: "worker", Dockerfile: "worker/Dockerfile"})
		}},
		{"same image", "", func(p *AppProfile) {
			p.Services = append(p.Services, Service{Name: "worker", Kind: "worker", Dockerfile: "./Dockerfile"})
		}},
		{"unresolved", "assumption traffic", func(p *AppProfile) {
			p.Assumptions = []Assumption{{ID: "traffic", Statement: "Confirm expected traffic"}}
		}},
		{"unrelated answer", "assumption traffic", func(p *AppProfile) {
			p.Assumptions = []Assumption{{ID: "traffic"}}
			p.Confirmed = []Confirmed{{ID: "other", Answer: "yes"}}
		}},
		{"empty answer", "assumption traffic", func(p *AppProfile) {
			p.Assumptions = []Assumption{{ID: "traffic"}}
			p.Confirmed = []Confirmed{{ID: "traffic", Answer: "  "}}
		}},
		{"resolved", "", func(p *AppProfile) {
			p.Assumptions = []Assumption{{ID: "traffic"}}
			p.Confirmed = []Confirmed{{ID: "traffic", Answer: "100 daily users"}}
		}},
		{"undecided database", "choose mode", func(p *AppProfile) { p.Needs = []Need{{Capability: "postgres", Mode: "undecided"}} }},
		{"reference needs connection name", "set secret_name", func(p *AppProfile) { p.Needs = []Need{{Capability: "postgres", Mode: "reference"}} }},
		{"unresolved redis", "unsupported redis", func(p *AppProfile) { p.Unsupported = []Unsupported{{Capability: "redis"}} }},
		{"external needs secret", "unsupported redis", func(p *AppProfile) {
			p.Unsupported = []Unsupported{{Capability: "redis", Disposition: "external", Resolution: "Existing Redis"}}
		}},
		{"not needed", "", func(p *AppProfile) {
			p.Unsupported = []Unsupported{{Capability: "redis", Disposition: "not-required", Resolution: "Used only in tests"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &AppProfile{Services: []Service{{Name: "web", Kind: "http", Dockerfile: "Dockerfile", Port: 3000, HealthPath: "/healthz"}}}
			tc.change(p)
			err := Ready(p)
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}
