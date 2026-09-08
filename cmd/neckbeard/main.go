// neckbeard core CLI. Deterministic by design: the agent plugin shells out to this
// binary; judgment lives in the agent, not here.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/catalog"
	"github.com/bealesh/neckbeard/core/config"
	"github.com/bealesh/neckbeard/core/planner"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/core/profile"
	"github.com/bealesh/neckbeard/core/version"
	"github.com/bealesh/neckbeard/schemas"
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
	case "plan":
		err = runPlan(os.Args[2:])
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
  plan     resolve app profile + config against the catalog into blueprint.yaml
  version  print version`)
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
