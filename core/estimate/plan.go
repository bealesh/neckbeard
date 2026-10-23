package estimate

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
)

// Corrections apply only to the catalog's billing and sizing configuration.
// A foreign or drifted plan must not be repriced using blueprint assumptions.
func validateGCPPlan(data []byte, env blueprint.Environment, expectedHTTP int) error {
	foundHTTP := 0
	type resource struct {
		Address, Type string
		Values        map[string]json.RawMessage
	}
	type module struct {
		Resources []resource        `json:"resources"`
		Children  []json.RawMessage `json:"child_modules"`
	}
	var plan struct {
		Planned struct {
			Root json.RawMessage `json:"root_module"`
		} `json:"planned_values"`
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		return fmt.Errorf("invalid plan JSON: %w", err)
	}
	if len(plan.Planned.Root) == 0 {
		return fmt.Errorf("plan JSON has no planned_values.root_module")
	}
	runtime := map[string]any{}
	for _, m := range env.Modules {
		if m.Name == "runtime-serverless" {
			for _, in := range m.Inputs {
				runtime[in.Key] = in.Value
			}
		}
	}
	var visit func(json.RawMessage) error
	visit = func(raw json.RawMessage) error {
		var m module
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		for _, r := range m.Resources {
			if strings.HasPrefix(r.Address, "module.postgres.google_sql_database_instance.") {
				var replicas []json.RawMessage
				if err := json.Unmarshal(r.Values["replica_configuration"], &replicas); err == nil && len(replicas) > 0 {
					return fmt.Errorf("plan %s has a replica configuration outside the catalog cost model", r.Address)
				}
			}
			if r.Type != "google_cloud_run_v2_service" || !strings.HasPrefix(r.Address, "module.runtime_serverless.") {
				continue
			}
			foundHTTP++
			var templates []struct {
				Scaling []struct {
					Min float64 `json:"min_instance_count"`
					Max float64 `json:"max_instance_count"`
				} `json:"scaling"`
				Containers []struct {
					Resources []struct {
						Idle   *bool             `json:"cpu_idle"`
						Limits map[string]string `json:"limits"`
					} `json:"resources"`
				} `json:"containers"`
			}
			if err := json.Unmarshal(r.Values["template"], &templates); err != nil || len(templates) != 1 || len(templates[0].Scaling) != 1 || len(templates[0].Containers) != 1 || len(templates[0].Containers[0].Resources) != 1 {
				return fmt.Errorf("plan %s has unresolved runtime sizing", r.Address)
			}
			template := templates[0]
			limits := template.Containers[0].Resources[0]
			if limits.Idle == nil || *limits.Idle {
				return fmt.Errorf("plan %s is not the catalog's instance-based billing configuration", r.Address)
			}
			cpu, err := planQuantity(limits.Limits["cpu"], true)
			if err != nil {
				return err
			}
			memory, err := planQuantity(limits.Limits["memory"], false)
			if err != nil {
				return err
			}
			for key, value := range map[string]float64{"cpu": cpu, "memory_gb": memory, "min_instances": template.Scaling[0].Min, "max_instances": template.Scaling[0].Max} {
				expected, err := inputNumber(runtime, key)
				if err != nil {
					return err
				}
				if math.Abs(value-expected) > 1e-9 {
					return fmt.Errorf("plan %s %s differs from blueprint: re-plan with the deployed sizing before estimating", r.Address, key)
				}
			}
			var name string
			_ = json.Unmarshal(r.Values["name"], &name)
			if prefix, ok := runtime["name_prefix"].(string); ok && !strings.HasPrefix(name, prefix+"-") {
				return fmt.Errorf("plan runtime does not belong to environment %s", env.Name)
			}
		}
		for _, child := range m.Children {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(plan.Planned.Root); err != nil {
		return err
	}
	if foundHTTP != expectedHTTP {
		return fmt.Errorf("plan contains %d catalog HTTP services, blueprint expects %d", foundHTTP, expectedHTTP)
	}
	return nil
}

func planQuantity(value string, cpu bool) (float64, error) {
	factor := 1.0
	if cpu {
		if strings.HasSuffix(value, "m") {
			value = strings.TrimSuffix(value, "m")
			factor = .001
		}
	} else {
		found := false
		for _, unit := range []struct {
			suffix string
			factor float64
		}{{"Gi", 1}, {"Mi", 1.0 / 1024}, {"Ki", 1.0 / (1024 * 1024)}, {"G", 1e9 / (1024 * 1024 * 1024)}, {"M", 1e6 / (1024 * 1024 * 1024)}} {
			if strings.HasSuffix(value, unit.suffix) {
				value = strings.TrimSuffix(value, unit.suffix)
				factor = unit.factor
				found = true
				break
			}
		}
		if !found {
			factor = 1.0 / (1024 * 1024 * 1024)
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0, fmt.Errorf("unresolved runtime CPU or memory quantity in plan")
	}
	return n * factor, nil
}
