// Package planner resolves app profile + configuration + tier presets against the
// pinned catalog into a deterministic blueprint. It is deliberately boring code:
// no I/O, no clock, no randomness — same inputs, same bytes (DESIGN §7, §9).
//
// The planner never invents infrastructure: every module comes from the catalog,
// every input from a preset, a derivation of config, or an allowlisted override.
package planner

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/catalog"
	"github.com/bealesh/neckbeard/core/config"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/core/profile"
)

type Inputs struct {
	Config        *config.Config
	ConfigDigest  string
	Profile       *profile.AppProfile
	ProfileDigest string
	Catalog       *catalog.Index
	Presets       *presets.Set
	// PlannerVersion is pinned into the blueprint so a rendering can always be
	// traced to the code that produced it.
	PlannerVersion string
}

func Plan(in Inputs) (*blueprint.Blueprint, error) {
	cfg, prof := in.Config, in.Profile

	cloud, ok := in.Catalog.Clouds[cfg.Cloud]
	if !ok {
		return nil, fmt.Errorf("catalog has no cloud %q (available: %s)", cfg.Cloud, keysCSV(in.Catalog.Clouds))
	}
	tier, ok := in.Presets.Tiers[cfg.Tier]
	if !ok {
		return nil, fmt.Errorf("no preset for tier %q (available: %s)", cfg.Tier, keysCSV(in.Presets.Tiers))
	}

	moduleNames, refs, warnings, err := requiredModules(cfg, prof, in.Catalog)
	if err != nil {
		return nil, err
	}

	// GCP and Azure need an explicit isolation container per environment; AWS
	// accounts are implied by the federated credentials.
	if cfg.Cloud != "aws" && len(cfg.Containers) == 0 {
		return nil, fmt.Errorf("cloud %q requires per-environment container ids (GCP project ids / Azure subscription ids); set containers: {dev: …, stg: …, prd: …} in neckbeard.yaml", cfg.Cloud)
	}

	// Overrides must target modules the plan actually uses and allowlisted inputs.
	if err := checkOverrides(cfg, cloud, moduleNames); err != nil {
		return nil, err
	}

	services := make([]blueprint.Service, 0, len(prof.Services))
	for _, s := range prof.Services {
		services = append(services, blueprint.Service{
			Name: s.Name, Kind: s.Kind, Port: s.Port, HealthPath: s.HealthPath, Schedule: s.Schedule,
			Dockerfile: s.Dockerfile,
		})
	}
	slices.SortFunc(services, func(a, b blueprint.Service) int { return strings.Compare(a.Name, b.Name) })

	// The secrets module provisions named secret containers (names only — values are
	// set out-of-band, never by neckbeard): the profile's declared secrets plus one
	// connection secret per referenced external service.
	secretNames := map[string]bool{}
	for _, s := range prof.Secrets {
		secretNames[s.Name] = true
	}
	for _, r := range refs {
		secretNames[r.SecretName] = true
	}

	var envs []blueprint.Environment
	for _, envName := range cfg.Environments {
		env := blueprint.Environment{Name: envName, Container: cfg.Containers[envName]}
		for _, modName := range moduleNames {
			mod, ok := cloud.Modules[modName]
			if !ok {
				return nil, fmt.Errorf("catalog cloud %q has no module %q required by this plan", cfg.Cloud, modName)
			}
			inputs := resolveInputs(cfg, tier, in.Presets, envName, modName, mod)
			if modName == "secrets" && len(secretNames) > 0 {
				inputs = insertInput(inputs, blueprint.Input{
					Key: "secret_names", Value: slices.Sorted(maps.Keys(secretNames)), Provenance: "derived",
				})
			}
			env.Modules = append(env.Modules, blueprint.ModuleUsage{
				Name:    modName,
				Source:  mod.Source,
				Version: mod.Version,
				Inputs:  inputs,
			})
		}
		envs = append(envs, env)
	}

	if cfg.Runtime == "kubernetes" {
		warnings = append(warnings, "kubernetes runtime: every environment gets a dedicated cluster, including dev — the dev preset uses small/spot nodes, but a dev control plane still bills while idle; see the cost estimate before scaffolding")
	}
	sort.Strings(warnings)

	bp := &blueprint.Blueprint{
		Version: 1,
		GeneratedFrom: blueprint.GeneratedFrom{
			NeckbeardYAML: in.ConfigDigest,
			AppProfile:    in.ProfileDigest,
		},
		Pins: blueprint.Pins{
			Catalog:   in.Catalog.Version,
			OpenTofu:  in.Catalog.OpenTofu,
			Planner:   in.PlannerVersion,
			Providers: providerPins(cloud),
		},
		App:          cfg.App,
		Org:          cfg.Org,
		Cloud:        cfg.Cloud,
		Region:       cfg.Region,
		Runtime:      cfg.Runtime,
		VCS:          cfg.VCS,
		Repo:         cfg.Repo,
		Tier:         cfg.Tier,
		Services:     services,
		Environments: envs,
		References:   refs,
		Warnings:     warnings,
	}
	return bp, nil
}

// requiredModules maps the profile onto catalog capabilities. Returns the sorted
// module list, external-service references, and warnings. Unknown capabilities are
// errors with the supported set named — never a nearest-fit.
func requiredModules(cfg *config.Config, prof *profile.AppProfile, idx *catalog.Index) ([]string, []blueprint.Reference, []string, error) {
	set := map[string]bool{}
	addCap := func(capability string) error {
		mods, ok := idx.CapabilityModules(cfg.Cloud, cfg.Runtime, capability)
		if !ok {
			return fmt.Errorf("capability %q is not in the catalog (supported: %s); neckbeard does not force-fit unsupported needs — see the workload contract in DESIGN §3", capability, keysCSV(idx.Capabilities))
		}
		for _, m := range mods {
			set[m] = true
		}
		return nil
	}

	if len(prof.Services) > 0 {
		if err := addCap("service-base"); err != nil {
			return nil, nil, nil, err
		}
	}
	for _, svc := range prof.Services {
		if svc.Kind == "http" {
			if err := addCap("http-ingress"); err != nil {
				return nil, nil, nil, err
			}
		}
	}

	var refs []blueprint.Reference
	var warnings []string
	for _, need := range prof.Needs {
		switch need.Mode {
		case "provision":
			if err := addCap(need.Capability); err != nil {
				return nil, nil, nil, err
			}
		case "reference":
			secretName := need.Capability + "-connection"
			refs = append(refs, blueprint.Reference{Capability: need.Capability, SecretName: secretName})
			warnings = append(warnings, fmt.Sprintf("%s is referenced, not provisioned: the app expects an existing service reachable via secret %q; neckbeard will not manage its lifecycle", need.Capability, secretName))
			// A referenced service still needs somewhere to hold its connection secret.
			if err := addCap("secrets"); err != nil {
				return nil, nil, nil, err
			}
		default:
			return nil, nil, nil, fmt.Errorf("need %q has unknown mode %q (provision|reference)", need.Capability, need.Mode)
		}
	}

	// Resolve the runtime placeholder to the configured runtime module.
	if set["runtime"] {
		delete(set, "runtime")
		if cfg.Runtime == "kubernetes" {
			set["runtime-k8s"] = true
		} else {
			set["runtime-serverless"] = true
		}
	}

	slices.SortFunc(refs, func(a, b blueprint.Reference) int { return strings.Compare(a.Capability, b.Capability) })
	return slices.Sorted(maps.Keys(set)), refs, warnings, nil
}

// resolveInputs layers, in order: tier preset → per-env adjustment → derived values
// from config → user overrides (already allowlist-checked). Later layers win; every
// value records its provenance. Output is sorted by key.
func resolveInputs(cfg *config.Config, tier presets.Tier, pre *presets.Set, envName, modName string, mod catalog.Module) []blueprint.Input {
	type resolved struct {
		value      any
		provenance string
	}
	merged := map[string]resolved{}

	for k, v := range tier.Modules[modName] {
		merged[k] = resolved{v, "preset"}
	}
	if envAdj, ok := pre.EnvAdjustments[envName]; ok {
		for k, v := range envAdj[modName] {
			merged[k] = resolved{v, "preset"}
		}
	}

	merged["name_prefix"] = resolved{fmt.Sprintf("%s-%s-%s", cfg.Org, cfg.App, envName), "derived"}
	merged["region"] = resolved{cfg.Region, "derived"}

	for k, v := range cfg.Overrides[modName] {
		merged[k] = resolved{v, "override-supported"}
	}

	inputs := make([]blueprint.Input, 0, len(merged))
	for _, k := range slices.Sorted(maps.Keys(merged)) {
		inputs = append(inputs, blueprint.Input{Key: k, Value: merged[k].value, Provenance: merged[k].provenance})
	}
	return inputs
}

// insertInput adds an input while preserving the sorted-by-key invariant.
func insertInput(inputs []blueprint.Input, in blueprint.Input) []blueprint.Input {
	i, _ := slices.BinarySearchFunc(inputs, in, func(a, b blueprint.Input) int { return strings.Compare(a.Key, b.Key) })
	return slices.Insert(inputs, i, in)
}

func checkOverrides(cfg *config.Config, cloud catalog.Cloud, activeModules []string) error {
	for _, modName := range slices.Sorted(maps.Keys(cfg.Overrides)) {
		if !slices.Contains(activeModules, modName) {
			return fmt.Errorf("override targets module %q, which this plan does not use (active modules: %s)", modName, strings.Join(activeModules, ", "))
		}
		mod := cloud.Modules[modName]
		for _, key := range slices.Sorted(maps.Keys(cfg.Overrides[modName])) {
			if !mod.AllowsOverride(key) {
				return fmt.Errorf("input %q on module %q is not overridable (allowed: %s); the golden-path promise only covers tested inputs — if you need this, open a catalog issue", key, modName, strings.Join(mod.Overridable, ", "))
			}
		}
	}
	return nil
}

func providerPins(cloud catalog.Cloud) []blueprint.Provider {
	out := make([]blueprint.Provider, 0, len(cloud.Providers))
	for _, p := range cloud.Providers {
		out = append(out, blueprint.Provider{Name: p.Name, Version: p.Version})
	}
	slices.SortFunc(out, func(a, b blueprint.Provider) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func keysCSV[V any](m map[string]V) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ", ")
}
