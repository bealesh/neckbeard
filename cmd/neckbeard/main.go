// neckbeard core CLI. Deterministic by design: the agent plugin shells out to this
// binary; judgment lives in the agent, not here.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bealesh/neckbeard/core/analyze"
	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/catalog"
	"github.com/bealesh/neckbeard/core/config"
	"github.com/bealesh/neckbeard/core/estimate"
	"github.com/bealesh/neckbeard/core/ownership"
	"github.com/bealesh/neckbeard/core/planner"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/core/profile"
	"github.com/bealesh/neckbeard/core/render"
	"github.com/bealesh/neckbeard/core/skillinstall"
	"github.com/bealesh/neckbeard/core/validate"
	"github.com/bealesh/neckbeard/core/version"
	"github.com/bealesh/neckbeard/schemas"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version":
		fmt.Println("neckbeard " + version.Version)
	case "analyze":
		err = runAnalyze(os.Args[2:])
	case "plan":
		err = runPlan(os.Args[2:])
	case "scaffold":
		err = runScaffold(os.Args[2:])
	case "validate":
		err = runValidate(os.Args[2:])
	case "estimate":
		err = runEstimate(os.Args[2:])
	case "skill":
		err = runSkill(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: neckbeard <command>

commands:
  analyze   deterministic repo detection → draft app-profile.yaml + open questions
  plan      resolve app profile + config against the catalog into blueprint.yaml
  estimate  cost report from the blueprint (rendered to scratch; no cloud creds)
  scaffold  render the blueprint into the repo under the ownership contract
  validate  V0 static checks over the rendered env roots (fmt, init, validate)
  skill     install the neckbeard agent skill (Codex CLI, Cursor, any SKILL.md tool)
  version   print version`)
}

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	dir := fs.String("dir", ".", "repository directory to analyze")
	out := fs.String("out", "app-profile.yaml", "draft profile output path")
	force := fs.Bool("force", false, "overwrite an existing profile (it is a reviewed input — prefer refining it)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil && !*force {
		return fmt.Errorf("%s already exists — it is a reviewed input (agent-written, user-corrected); refine it in place or pass -force to start over", *out)
	}

	res, err := analyze.Dir(*dir)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(res.Profile)
	if err != nil {
		return err
	}
	header := "# DRAFT app profile — deterministic detection only (facts carry evidence;\n# assumptions are loud). Refine with judgment, answer the open questions into\n# `confirmed:`, then run `neckbeard plan`.\n"
	if err := os.WriteFile(*out, append([]byte(header), data...), 0o644); err != nil {
		return err
	}

	fmt.Printf("draft written: %s (%d services, %d needs, %d secrets, %d facts, %d assumptions, %d unsupported)\n\n",
		*out, len(res.Profile.Services), len(res.Profile.Needs), len(res.Profile.Secrets), len(res.Profile.Facts), len(res.Profile.Assumptions), len(res.Profile.Unsupported))
	fmt.Println("OPEN QUESTIONS — inspection cannot settle these; record answers under confirmed:")
	for i, q := range res.Questions {
		fmt.Printf("  %d. %s\n", i+1, q)
	}
	return nil
}

func runPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	configPath := fs.String("config", "neckbeard.yaml", "path to neckbeard.yaml")
	profilePath := fs.String("profile", "app-profile.yaml", "path to app-profile.yaml")
	catalogPath := fs.String("catalog", "catalog/index.yaml", "path to catalog index")
	outPath := fs.String("out", "blueprint.yaml", "output blueprint path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, cfgDigest, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", *configPath, err)
	}
	prof, profDigest, err := profile.Load(*profilePath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", *profilePath, err)
	}
	cat, err := catalog.Load(*catalogPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", *catalogPath, err)
	}
	pre, err := presets.Load()
	if err != nil {
		return err
	}

	bp, err := planner.Plan(planner.Inputs{
		Config:         cfg,
		ConfigDigest:   cfgDigest,
		Profile:        prof,
		ProfileDigest:  profDigest,
		Catalog:        cat,
		Presets:        pre,
		PlannerVersion: version.Version,
	})
	if err != nil {
		return err
	}
	out, err := bp.Finalize()
	if err != nil {
		return err
	}
	// Self-check: the planner's own output must satisfy the published contract.
	if err := schemas.ValidateYAML("blueprint", out); err != nil {
		return fmt.Errorf("internal error: generated blueprint violates its schema: %w", err)
	}
	if err := os.WriteFile(*outPath, out, 0o644); err != nil {
		return err
	}

	printSummary(bp, *outPath)
	return nil
}

func runScaffold(args []string) error {
	fs := flag.NewFlagSet("scaffold", flag.ExitOnError)
	blueprintPath := fs.String("blueprint", "blueprint.yaml", "path to blueprint.yaml")
	root := fs.String("root", ".", "repository root to scaffold into")
	catalogSource := fs.String("catalog-source", render.DefaultCatalogSource, "module source base: a git go-getter base or a local path (dev)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	bp, err := blueprint.Load(*blueprintPath)
	if err != nil {
		return err
	}
	ws, err := render.WriteSet(bp, render.Options{CatalogSource: *catalogSource})
	if err != nil {
		return err
	}
	res, err := ownership.Apply(*root, bp.Hash, ws)
	if err != nil {
		return err
	}

	report := func(label string, paths []string) {
		for _, p := range paths {
			fmt.Printf("%-10s %s\n", label, p)
		}
	}
	report("created", res.Created)
	report("updated", res.Updated)
	report("unchanged", res.Unchanged)
	report("adopted", res.Adopted)
	report("yours", res.SkippedUser)
	report("removed", res.RemovedStale)
	report("kept", res.KeptStale)

	if res.HasConflicts() {
		fmt.Println()
		for _, c := range res.Conflicts {
			fmt.Printf("conflict   %s — %s; fresh render written to %s\n", c.Path, c.Reason, c.NewPath)
		}
		return fmt.Errorf("%d conflict(s): neckbeard never overwrites files it did not just generate — review each <file>%s diff, then either keep your edit (move it to an override or a custom.tf extension point) or replace the file with the %s version and re-run scaffold", len(res.Conflicts), ownership.NewSuffix, ownership.NewSuffix)
	}
	fmt.Printf("\nscaffold complete (blueprint %s)\n", bp.Hash)
	return nil
}

func runEstimate(args []string) error {
	fs := flag.NewFlagSet("estimate", flag.ExitOnError)
	configPath := fs.String("config", "neckbeard.yaml", "path to neckbeard.yaml")
	blueprintPath := fs.String("blueprint", "blueprint.yaml", "path to blueprint.yaml")
	catalogSource := fs.String("catalog-source", render.DefaultCatalogSource, "module source base (local path speeds estimation up)")
	infracostBin := fs.String("infracost-bin", "", "infracost 0.10.x binary (default: infracost-0.10, then infracost, from PATH)")
	outPath := fs.String("out", "costs/estimate.md", "report output path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, cfgDigest, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", *configPath, err)
	}
	bp, err := blueprint.Load(*blueprintPath)
	if err != nil {
		return err
	}

	res, err := estimate.Run(estimate.Options{
		Config: cfg, ConfigDigest: cfgDigest, Blueprint: bp,
		CatalogSource: *catalogSource, InfracostBin: *infracostBin,
	})
	if err != nil {
		return err
	}
	report := estimate.Report(res, cfg, bp, time.Now())
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*outPath, report, 0o644); err != nil {
		return err
	}

	for _, e := range res.Envs {
		fmt.Printf("env %-4s baseline %8.2f   expected %8.2f   range %.2f–%.2f %s/mo\n",
			e.Env+":", e.Costs["baseline"], e.Costs["expected"], e.Costs["low"], e.Costs["high"], res.Currency)
	}
	var total float64
	for _, e := range res.Envs {
		total += e.Costs["expected"]
	}
	fmt.Printf("\nexpected total: %.2f %s/mo (%.2f/yr) — full report: %s\n", total, res.Currency, total*12, *outPath)
	fmt.Println("estimates ride on section 2's usage assumptions; budget alerts notify, they don't cap")
	return nil
}

func runSkill(args []string) error {
	if len(args) < 1 || args[0] != "install" {
		return fmt.Errorf("usage: neckbeard skill install [-target agents|codex|cursor] [-scope project|user]")
	}
	fs := flag.NewFlagSet("skill install", flag.ExitOnError)
	target := fs.String("target", "agents", "agents (open standard: Codex, Cursor, and others read it) | codex | cursor")
	scope := fs.String("scope", "project", "project (this repo) | user (your home directory)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	root := "."
	if *scope == "user" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		root = home
	} else if *scope != "project" {
		return fmt.Errorf("scope must be project or user")
	}

	written, err := skillinstall.Install(root, skillinstall.Target(*target))
	if err != nil {
		return err
	}
	for _, p := range written {
		fmt.Printf("installed  %s\n", filepath.Join(root, p))
	}
	fmt.Println("\nInvoke it from your agent (Codex: /skills or $neckbeard; Cursor: the skill loads on demand, /neckbeard-analyze for the command).")
	fmt.Println("Claude Code users: /plugin marketplace add bealesh/neckbeard && /plugin install neckbeard@neckbeard")
	return nil
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	blueprintPath := fs.String("blueprint", "blueprint.yaml", "path to blueprint.yaml")
	root := fs.String("root", ".", "repository root containing infra/envs/")
	if err := fs.Parse(args); err != nil {
		return err
	}

	bp, err := blueprint.Load(*blueprintPath)
	if err != nil {
		return err
	}
	envs := make([]string, 0, len(bp.Environments))
	for _, e := range bp.Environments {
		envs = append(envs, e.Name)
	}

	checks, err := validate.StaticV0(*root, envs)
	if err != nil {
		return err
	}
	fmt.Printf("%-3s  %-13s  %-32s  %s\n", "LVL", "STATUS", "CHECK", "DETAIL")
	for _, c := range checks {
		name := c.Name
		if c.Env != "" {
			name = c.Name + " [" + c.Env + "]"
		}
		fmt.Printf("%-3s  %-13s  %-32s  %s\n", c.Level, c.Status, name, c.Detail)
	}
	if validate.AnyFailed(checks) {
		return fmt.Errorf("V0 static validation failed")
	}
	fmt.Println("\nV0 static checks passed. Passing V0 proves syntax, schema, and policy conformance only — not deployability or runtime behavior (V1/V2 not exercised).")
	return nil
}

// printSummary shows topology consequences before anything is generated (DESIGN §4.2).
func printSummary(bp *blueprint.Blueprint, outPath string) {
	fmt.Printf("app:   %s (org %s)\n", bp.App, bp.Org)
	fmt.Printf("lane:  %s / %s / %s / tier %s / region %s\n", bp.Cloud, bp.VCS, bp.Runtime, bp.Tier, bp.Region)
	fmt.Printf("pins:  catalog %s, opentofu %s, planner %s\n\n", bp.Pins.Catalog, bp.Pins.OpenTofu, bp.Pins.Planner)
	for _, env := range bp.Environments {
		names := make([]string, 0, len(env.Modules))
		for _, m := range env.Modules {
			names = append(names, m.Name)
		}
		fmt.Printf("env %-4s %d modules: %s\n", env.Name+":", len(env.Modules), strings.Join(names, ", "))
	}
	if len(bp.References) > 0 {
		fmt.Println()
		for _, r := range bp.References {
			fmt.Printf("reference: %s via secret %q (existing service; not provisioned)\n", r.Capability, r.SecretName)
		}
	}
	if len(bp.Warnings) > 0 {
		fmt.Println()
		for _, w := range bp.Warnings {
			fmt.Println("warning: " + w)
		}
	}
	fmt.Printf("\nblueprint: %s (%s)\n", outPath, bp.Hash)
	fmt.Println("next: `neckbeard estimate` to see cost consequences before scaffolding")
}
