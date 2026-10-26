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
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/config"
	"github.com/bealesh/neckbeard/core/doctor"
	"github.com/bealesh/neckbeard/core/ownership"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/core/render"
)

type Options struct {
	Root          string // application root for user-owned Terraform cost inputs
	PlanPath      string // optional authenticated plan JSON, local only
	Environment   string // restrict a plan or report to one environment
	Config        *config.Config
	ConfigDigest  string
	Blueprint     *blueprint.Blueprint
	CatalogSource string // defaults to bundled modules, copied into scratch
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
	forwardingRules int
	Env             string
	Costs           map[string]float64 // scenario name → monthly cost
	Resources       []ResourceCost     // expected scenario, sorted desc
	Unpriced        []string           // resource types/components not separately priced
	Unknown         []string           // billable components missing a price or quantity
	Adjustments     []string           // service-level corrections to the upstream adapter
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
	InputDigest      string
	Scope            string
	Source           string
}

func Run(opts Options) (*Result, error) {
	// Discovery order: explicit flag, then the classic CLI installed as
	// infracost-0.10 (the v2 SaaS CLI often shadows `infracost`), then PATH.
	binPath, err := doctor.Infracost(opts.InfracostBin)
	if err != nil {
		return nil, fmt.Errorf("infracost not found: install the 0.10.x CLI (as `infracost-0.10` or `infracost`) and run its `auth login`, or pass -infracost-bin")
	}
	version, err := infracostVersion(binPath)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.TrimPrefix(version, "v"), "0.10.") {
		return nil, fmt.Errorf("infracost %s is not the supported classic CLI; neckbeard estimates with the 0.10.x CLI — point -infracost-bin at it", version)
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

	inputs, err := deploymentInputs(opts.Root)
	if err != nil {
		return nil, err
	}
	generated := map[string]bool{}
	for _, file := range ws {
		if file.Owner == ownership.OwnerGenerated {
			generated[file.Path] = true
		}
	}
	for path, data := range inputs {
		if generated[path] {
			return nil, fmt.Errorf("local Terraform input %s collides with generated output; resolve ownership before estimating", path)
		}
		full := filepath.Join(scratch, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err = os.WriteFile(full, data, 0o600); err != nil {
			return nil, err
		}
	}
	res := &Result{Usage: usage, UsageSource: usageSource, InfracostVersion: version, Currency: currencyOr(opts.Config), InputDigest: inputDigest(opts.Blueprint.Hash, inputs), Scope: "blueprint", Source: "effective HCL"}
	lookup, err := pricingLookup(binPath, res.Currency)
	if err != nil {
		return nil, err
	}
	if opts.PlanPath != "" {
		res.Source = "authenticated plan JSON"
		data, err := os.ReadFile(opts.PlanPath)
		if err != nil {
			return nil, err
		}
		inputs["authenticated-plan.json"] = data
		res.InputDigest = inputDigest(opts.Blueprint.Hash, inputs)
	}
	if opts.PlanPath != "" && opts.Environment == "" {
		return nil, fmt.Errorf("-plan requires -env so a plan cannot be mistaken for all environments")
	}
	if opts.Environment != "" {
		res.Scope = opts.Environment
	}

	mapped := map[string]bool{}
	projectRules := map[string]float64{}
	for _, env := range opts.Blueprint.Environments {
		if opts.Environment != "" && env.Name != opts.Environment {
			continue
		}
		envDir := filepath.Join(scratch, "infra", "envs", env.Name)
		if opts.PlanPath != "" {
			envDir = opts.PlanPath
			if err := validateCostPlan(inputs["authenticated-plan.json"], opts.Blueprint, env); err != nil {
				return nil, err
			}
		}
		project := env.Container
		if project == "" {
			project = env.Name
		}
		envUsage := usage
		if opts.Blueprint.Cloud == "gcp" {
			envUsage = map[string]float64{}
			for k, v := range usage {
				envUsage[k] = v
			}
			existing, ok := projectRules[project]
			if !ok {
				existing = usage["existing_global_forwarding_rules"]
			}
			envUsage["existing_global_forwarding_rules"] = existing
		}
		ee, types, err := estimateEnv(binPath, scratch, envDir, env, opts.Blueprint, res.Currency, envUsage, lookup)
		if err != nil {
			return nil, fmt.Errorf("estimating %s: %w", env.Name, err)
		}
		if opts.Blueprint.Cloud == "gcp" {
			projectRules[project] = envUsage["existing_global_forwarding_rules"] + float64(ee.forwardingRules)
			mapped["log_ingest_gb"] = true
			if types["google_cloud_run_v2_service"] {
				mapped["active_instance_hours"] = true
			}
			if types["google_compute_global_forwarding_rule"] {
				for _, k := range []string{"ingress_gb", "egress_gb", "existing_global_forwarding_rules"} {
					mapped[k] = true
				}
			}
		}
		if opts.Blueprint.Runtime == "kubernetes" {
			mapped["additional_node_hours"] = true
		}
		if opts.Blueprint.Runtime == "serverless-containers" {
			for _, svc := range opts.Blueprint.Services {
				if svc.Kind == "http" {
					mapped["active_instance_hours"] = true
				}
				if svc.Kind == "cron" {
					mapped["job_executions_per_month"] = true
					mapped["job_duration_minutes"] = true
				}
			}
		}
		mapped["egress_gb"] = true
		mapped["ingress_gb"] = true
		mapped["cross_zone_gb"] = true
		if opts.Blueprint.Cloud == "aws" {
			mapped["log_ingest_gb"] = true
			mapped["log_query_gb"] = true
			if opts.Blueprint.Runtime == "serverless-containers" {
				mapped["custom_metrics"] = true
			}
		}
		if opts.Blueprint.Cloud == "azure" || opts.Blueprint.Cloud == "aws" {
			for _, svc := range opts.Blueprint.Services {
				if svc.Kind == "http" {
					mapped["requests_per_month"] = true
				}
			}
		}
		for k := range AppliedUsage(types) {
			mapped[k] = true
		}
		res.Envs = append(res.Envs, *ee)
	}
	if len(res.Envs) == 0 {
		return nil, fmt.Errorf("environment %q is not in the blueprint", opts.Environment)
	}
	for k := range usage {
		if !mapped[k] {
			res.UnmappedUsage = append(res.UnmappedUsage, k)
		}
	}
	sortStrings(res.UnmappedUsage)
	return res, nil
}

func estimateEnv(bin, scratch, envDir string, env blueprint.Environment, bp *blueprint.Blueprint, currency string, usage map[string]float64, lookup productLookup) (*EnvEstimate, map[string]bool, error) {
	usageFile := filepath.Join(scratch, "usage-"+env.Name+".yml")
	ee := &EnvEstimate{Env: env.Name, Costs: map[string]float64{}}
	types := map[string]bool{}
	for _, sc := range scenarios {
		uf, err := BuildUsageFile(usage, sc.Multiplier)
		if err != nil {
			return nil, nil, fmt.Errorf("building usage file: %w", err)
		}
		if err := os.WriteFile(usageFile, uf, 0o644); err != nil {
			return nil, nil, err
		}
		out, err := runInfracost(bin, currency, "breakdown", "--path", envDir, "--format", "json", "--usage-file", usageFile)
		if err != nil {
			return nil, nil, fmt.Errorf("scenario %s: %w", sc.Name, err)
		}
		var adjustments []string
		{
			var doc breakdown
			if err = json.Unmarshal(out, &doc); err != nil {
				return nil, nil, fmt.Errorf("invalid pricing output: %w", err)
			}
			if _, err = number(doc.TotalMonthlyCost, "total monthly cost"); err != nil {
				return nil, nil, err
			}
			adjustments, err = correctCloud(&doc, bp, env, usage, sc.Multiplier, lookup)
			if err != nil {
				return nil, nil, err
			}
			if sc.Name == "expected" {
				for _, p := range doc.Projects {
					for _, r := range p.Breakdown.Resources {
						if r == nil {
							return nil, nil, fmt.Errorf("null resource in pricing output")
						}
						if isGCPIngressRule(r) {
							ee.forwardingRules++
						}
					}
				}
			}
			out, err = json.Marshal(doc)
			if err != nil {
				return nil, nil, err
			}
		}
		total, resources, resourceTypes, unpriced, err := parseBreakdown(out)
		if err != nil {
			return nil, nil, fmt.Errorf("scenario %s: %w", sc.Name, err)
		}
		ee.Costs[sc.Name] = total
		if sc.Name == "expected" {
			ee.Resources = resources
			ee.Unpriced = unpriced
			ee.Adjustments = adjustments
			for _, item := range unpriced {
				if strings.HasSuffix(item, "(missing price or usage quantity)") || strings.HasSuffix(item, "(unsupported by infracost)") {
					ee.Unknown = append(ee.Unknown, item)
				}
			}
			types = resourceTypes
		}
	}
	return ee, types, nil
}

func runInfracost(bin, currency string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	// The requested currency is passed to the pricing API — never applied as a
	// label to USD numbers (review finding, 2026-09-08).
	cmd.Env = append(os.Environ(), "INFRACOST_SKIP_UPDATE_CHECK=true", "INFRACOST_NO_COLOR=true", "INFRACOST_CURRENCY="+currency)
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

func infracostVersion(bin string) (string, error) { return doctor.Version(bin) }

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
		if !strings.HasPrefix(f.Path, "infra/envs/") && !strings.HasPrefix(f.Path, ".neckbeard/catalog/") {
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

// Complete means every returned billable component has a price and quantity.
func (r *Result) Complete() bool {
	if len(r.Envs) == 0 {
		return false
	}
	for _, e := range r.Envs {
		if len(e.Unknown) > 0 {
			return false
		}
	}
	return true
}
