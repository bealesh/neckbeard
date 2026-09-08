// Package estimate implements the FinOps pipeline (DESIGN §13): the blueprint is
// rendered to a scratch directory (never the user's repo) and priced with
// Infracost's HCL parsing — no cloud credentials involved. Costs are reported in
// five honest sections: baseline provisioned, usage assumptions (explicit
// numbers — a tier name is not a usage estimate), scenario ranges, shared
// platform costs, and unpriced items. Budget alerts notify; they do not cap.
package estimate

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/config"
	"github.com/bealesh/neckbeard/core/ownership"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/core/render"
)

type Options struct {
	Config        *config.Config
	ConfigDigest  string
	Blueprint     *blueprint.Blueprint
	CatalogSource string // local path preferred: estimation renders to scratch
	InfracostBin  string
}

// Scenario multipliers over the expected usage numbers (DESIGN §13.2: ranges, not
// one false-precision figure).
var scenarios = []struct {
	Name       string
	Multiplier float64
}{
	{"baseline", 0}, // usage zeroed: only resources that bill while idle
	{"low", 0.5},
	{"expected", 1},
	{"high", 2},
}

type EnvEstimate struct {
	Env       string
	Costs     map[string]float64 // scenario name → monthly cost
	Resources []ResourceCost     // expected scenario, sorted desc
	Unpriced  []string           // resource types infracost could not price
}

type ResourceCost struct {
	Address string
	Monthly float64
}

type Result struct {
	Envs             []EnvEstimate
	Usage            map[string]float64 // resolved usage assumptions (tier + overrides)
	UsageSource      map[string]string  // usage key → "tier preset" | "finops.usage override"
	UnmappedUsage    []string           // assumptions with no IaC resource to attach to
	InfracostVersion string
	Currency         string
}

func Run(opts Options) (*Result, error) {
	bin := opts.InfracostBin
	if bin == "" {
		bin = "infracost"
	}
	binPath, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("infracost not found (%q): install the 0.10.x CLI and run `infracost auth login`, or pass -infracost-bin", bin)
	}
	version, err := infracostVersion(binPath)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(version, "v2.") || strings.HasPrefix(version, "2.") {
		return nil, fmt.Errorf("infracost %s is the v2 SaaS CLI (requires a dashboard organization and replaces breakdown with scan); neckbeard estimates with the 0.10.x CLI — point -infracost-bin at it", version)
	}

	// Staleness gate: the blueprint must have been planned from this exact config.
	if opts.Blueprint.GeneratedFrom.NeckbeardYAML != opts.ConfigDigest {
		return nil, fmt.Errorf("blueprint was generated from a different neckbeard.yaml (recorded %s, current %s): re-run `neckbeard plan` first", opts.Blueprint.GeneratedFrom.NeckbeardYAML, opts.ConfigDigest)
	}

	pre, err := presets.Load()
	if err != nil {
		return nil, err
	}
	tier, ok := pre.Tiers[opts.Blueprint.Tier]
	if !ok {
		return nil, fmt.Errorf("no preset for tier %q", opts.Blueprint.Tier)
	}
	usage, usageSource := resolveUsage(tier, opts.Config)

	// Render to scratch (§13.1): estimation must work before anything is scaffolded
	// into the user's repository.
	scratch, err := os.MkdirTemp("", "neckbeard-estimate-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	ws, err := render.WriteSet(opts.Blueprint, render.Options{CatalogSource: opts.CatalogSource})
	if err != nil {
		return nil, err
	}
	if err := writeScratch(scratch, ws); err != nil {
		return nil, err
	}

	res := &Result{Usage: usage, UsageSource: usageSource, InfracostVersion: version, Currency: currencyOr(opts.Config)}
	mapped := map[string]bool{}
	for _, env := range opts.Blueprint.Environments {
		envDir := filepath.Join(scratch, "infra", "envs", env.Name)
		ee, envMapped, err := estimateEnv(binPath, scratch, envDir, env.Name, usage)
		if err != nil {
			return nil, fmt.Errorf("estimating %s: %w", env.Name, err)
		}
		for k := range envMapped {
			mapped[k] = true
		}
		res.Envs = append(res.Envs, *ee)
	}
	for k := range usage {
		if !mapped[k] {
			res.UnmappedUsage = append(res.UnmappedUsage, k)
		}
	}
	sortStrings(res.UnmappedUsage)
	return res, nil
}

func estimateEnv(bin, scratch, envDir, envName string, usage map[string]float64) (*EnvEstimate, map[string]bool, error) {
	usageFile := filepath.Join(scratch, "usage-"+envName+".yml")
	// Sync writes a skeleton with every resource address and its usage keys zeroed
	// — the source of truth for which keys exist, instead of guessing.
	if _, err := runInfracost(bin, "breakdown", "--path", envDir, "--format", "json",
		"--sync-usage-file", "--usage-file", usageFile); err != nil {
		return nil, nil, fmt.Errorf("usage sync: %w", err)
	}
	skeleton, err := os.ReadFile(usageFile)
	if err != nil {
		return nil, nil, err
	}

	ee := &EnvEstimate{Env: envName, Costs: map[string]float64{}}
	var mapped map[string]bool
	for _, sc := range scenarios {
		filled, m, err := FillUsage(skeleton, usage, sc.Multiplier)
		if err != nil {
			return nil, nil, fmt.Errorf("filling usage file: %w", err)
		}
		mapped = m
		if err := os.WriteFile(usageFile, filled, 0o644); err != nil {
			return nil, nil, err
		}
		out, err := runInfracost(bin, "breakdown", "--path", envDir, "--format", "json", "--usage-file", usageFile)
		if err != nil {
			return nil, nil, fmt.Errorf("scenario %s: %w", sc.Name, err)
		}
		total, resources, unpriced, err := parseBreakdown(out)
		if err != nil {
			return nil, nil, fmt.Errorf("scenario %s: %w", sc.Name, err)
		}
		ee.Costs[sc.Name] = total
		if sc.Name == "expected" {
			ee.Resources = resources
			ee.Unpriced = unpriced
		}
	}
	return ee, mapped, nil
}

func runInfracost(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "INFRACOST_SKIP_UPDATE_CHECK=true", "INFRACOST_NO_COLOR=true")
	out, err := cmd.Output()
	if err != nil {
		detail := ""
		if ee, ok := err.(*exec.ExitError); ok {
			detail = strings.TrimSpace(string(ee.Stderr))
			if len(detail) > 400 {
				detail = detail[len(detail)-400:]
			}
		}
		return nil, fmt.Errorf("infracost %s: %w (%s)", args[0], err, detail)
	}
	return out, nil
}

func infracostVersion(bin string) (string, error) {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("infracost --version: %w", err)
	}
	fields := strings.Fields(string(out))
	return fields[len(fields)-1], nil
}

// parseBreakdown extracts the totals from infracost's JSON output.
func parseBreakdown(out []byte) (total float64, resources []ResourceCost, unpriced []string, err error) {
	var doc struct {
		TotalMonthlyCost string `json:"totalMonthlyCost"`
		Projects         []struct {
			Breakdown struct {
				Resources []struct {
					Name        string  `json:"name"`
					MonthlyCost *string `json:"monthlyCost"`
				} `json:"resources"`
			} `json:"breakdown"`
		} `json:"projects"`
		Summary struct {
			UnsupportedResourceCounts map[string]int `json:"unsupportedResourceCounts"`
			NoPriceResourceCounts     map[string]int `json:"noPriceResourceCounts"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return 0, nil, nil, fmt.Errorf("parsing infracost JSON: %w", err)
	}
	total, _ = strconv.ParseFloat(doc.TotalMonthlyCost, 64)
	for _, p := range doc.Projects {
		for _, r := range p.Breakdown.Resources {
			cost := 0.0
			if r.MonthlyCost != nil {
				cost, _ = strconv.ParseFloat(*r.MonthlyCost, 64)
			}
			resources = append(resources, ResourceCost{Address: r.Name, Monthly: cost})
		}
	}
	sortResources(resources)
	for t, n := range doc.Summary.UnsupportedResourceCounts {
		unpriced = append(unpriced, fmt.Sprintf("%s ×%d (unsupported by infracost)", t, n))
	}
	for t, n := range doc.Summary.NoPriceResourceCounts {
		unpriced = append(unpriced, fmt.Sprintf("%s ×%d (free / no price)", t, n))
	}
	sortStrings(unpriced)
	return total, resources, unpriced, nil
}

func resolveUsage(tier presets.Tier, cfg *config.Config) (map[string]float64, map[string]string) {
	usage := map[string]float64{}
	source := map[string]string{}
	for k, v := range tier.Usage {
		usage[k] = v
		source[k] = "tier preset (" + cfg.Tier + ")"
	}
	for k, v := range cfg.FinOps.Usage {
		usage[k] = v
		source[k] = "finops.usage override"
	}
	return usage, source
}

func writeScratch(root string, files []ownership.File) error {
	for _, f := range files {
		if !strings.HasPrefix(f.Path, "infra/envs/") {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, f.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func currencyOr(cfg *config.Config) string {
	if cfg.FinOps.Currency != "" {
		return cfg.FinOps.Currency
	}
	return "USD"
}

func sortResources(rs []ResourceCost) {
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].Monthly > rs[j].Monthly })
}

func sortStrings(s []string) {
	sort.Strings(s)
}
