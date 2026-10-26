package estimate

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
)

type costPlanResource struct {
	Address, Type string
	Values        map[string]any
}
type costPlanModule struct {
	Resources []costPlanResource
	Children  []costPlanModule `json:"child_modules"`
}

func planField(value any, path string) any {
	for _, key := range strings.Split(path, ".") {
		switch v := value.(type) {
		case map[string]any:
			value = v[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(v) {
				return nil
			}
			value = v[index]
		default:
			return nil
		}
	}
	return value
}

func checkPlanField(r costPlanResource, path string, want any) error {
	actual := planField(r.Values, path)
	if actual == nil {
		return fmt.Errorf("plan %s has unresolved cost input %s", r.Address, path)
	}
	matches := fmt.Sprint(actual) == fmt.Sprint(want)
	if n, ok := want.(float64); ok {
		got, err := number(fmt.Sprint(actual), "plan quantity")
		matches = err == nil && math.Abs(got-n) < 1e-9
	}
	if !matches {
		return fmt.Errorf("plan %s %s differs from the catalog cost inputs: re-plan with the deployed settings before estimating", r.Address, path)
	}
	return nil
}

func validateCostPlan(data []byte, bp *blueprint.Blueprint, env blueprint.Environment) error {
	if bp.Cloud == "gcp" {
		expectedHTTP := 0
		if bp.Runtime == "serverless-containers" {
			for _, svc := range bp.Services {
				if svc.Kind == "http" {
					expectedHTTP++
				}
			}
		}
		if err := validateGCPPlan(data, env, expectedHTTP); err != nil {
			return err
		}
	}
	var doc struct {
		Planned struct {
			Root *costPlanModule `json:"root_module"`
		} `json:"planned_values"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("invalid plan JSON: %w", err)
	}
	if doc.Planned.Root == nil {
		return fmt.Errorf("plan JSON has no planned_values.root_module")
	}
	serverless := moduleInputs(env, "runtime-serverless")
	nodes := moduleInputs(env, "runtime-k8s")
	var visit func(costPlanModule) error
	visit = func(m costPlanModule) error {
		for _, r := range m.Resources {
			expected := map[string]any{}
			if strings.HasPrefix(r.Address, "module.runtime_serverless.") {
				cpu, _ := inputNumber(serverless, "cpu")
				memory, _ := inputNumber(serverless, "memory_gb")
				floor, _ := inputNumber(serverless, "min_instances")
				maximum, _ := inputNumber(serverless, "max_instances")
				switch r.Type {
				case "aws_ecs_task_definition":
					expected["cpu"] = cpu * 1024
					expected["memory"] = memory * 1024
				case "aws_appautoscaling_target":
					expected["min_capacity"] = math.Max(1, floor)
					expected["max_capacity"] = maximum
				case "aws_ecs_service":
					for _, svc := range bp.Services {
						if svc.Kind == "worker" && strings.Contains(r.Address, fmt.Sprintf("[%q]", svc.Name)) {
							expected["desired_count"] = math.Max(1, floor)
						}
					}
				case "azurerm_container_app", "azurerm_container_app_job":
					expected["template.0.container.0.cpu"] = cpu
					expected["template.0.container.0.memory"] = fmt.Sprint(memory) + "Gi"
					if r.Type == "azurerm_container_app" {
						minimum := floor
						for _, svc := range bp.Services {
							if svc.Kind == "worker" && strings.Contains(r.Address, fmt.Sprintf("[%q]", svc.Name)) {
								minimum = math.Max(1, minimum)
							}
						}
						expected["template.0.min_replicas"] = minimum
						expected["template.0.max_replicas"] = maximum
					}
				case "azurerm_container_app_environment":
					profiles, _ := r.Values["workload_profile"].([]any)
					if len(profiles) != 1 {
						return fmt.Errorf("plan environment is not the catalog's Consumption-only profile")
					}
					expected["workload_profile.0.workload_profile_type"] = "Consumption"
				}
			}
			if strings.HasPrefix(r.Address, "module.runtime_k8s.") {
				min, _ := inputNumber(nodes, "min_nodes")
				max, _ := inputNumber(nodes, "max_nodes")
				spot, _ := nodes["spot"].(bool)
				shape := fmt.Sprint(nodes["node_shape"])
				switch r.Type {
				case "aws_eks_node_group":
					machine := "t3a.medium"
					if shape == "medium" {
						machine = "m6a.large"
					}
					expected["instance_types.0"] = machine
					expected["scaling_config.0.min_size"] = min
					expected["scaling_config.0.max_size"] = max
					capacity := "ON_DEMAND"
					if spot {
						capacity = "SPOT"
					}
					expected["capacity_type"] = capacity
					if disk := r.Values["disk_size"]; disk != nil {
						expected["disk_size"] = float64(20)
					}
					if ami := r.Values["ami_type"]; ami != nil {
						expected["ami_type"] = "AL2023_x86_64_STANDARD"
					}
					if templates, _ := r.Values["launch_template"].([]any); len(templates) > 0 {
						return fmt.Errorf("custom EKS launch-template disks are outside the catalog cost model")
					}
				case "google_container_node_pool":
					machine := "e2-medium"
					if shape == "medium" {
						machine = "e2-standard-4"
					}
					expected["node_config.0.machine_type"] = machine
					expected["node_config.0.spot"] = spot
					expected["autoscaling.0.total_min_node_count"] = min
					expected["autoscaling.0.total_max_node_count"] = max
					if disk := planField(r.Values, "node_config.0.disk_type"); disk != nil && disk != "" {
						expected["node_config.0.disk_type"] = "pd-balanced"
					}
					if disk := planField(r.Values, "node_config.0.disk_size_gb"); disk != nil {
						expected["node_config.0.disk_size_gb"] = float64(100)
					}
				case "azurerm_kubernetes_cluster", "azurerm_kubernetes_cluster_node_pool":
					machine := "Standard_D2as_v5"
					if shape == "medium" {
						machine = "Standard_D4as_v5"
					}
					prefix := ""
					if r.Type == "azurerm_kubernetes_cluster" {
						prefix = "default_node_pool.0."
						expected["sku_tier"] = "Standard"
					}
					expected[prefix+"vm_size"] = machine
					if r.Type == "azurerm_kubernetes_cluster" && spot {
						expected[prefix+"node_count"] = float64(1)
					} else {
						expected[prefix+"min_count"] = min
						expected[prefix+"max_count"] = max
					}
					if r.Type == "azurerm_kubernetes_cluster_node_pool" {
						expected["priority"] = "Spot"
					}
					if size := planField(r.Values, prefix+"os_disk_size_gb"); size != nil && fmt.Sprint(size) != "0" {
						expected[prefix+"os_disk_size_gb"] = float64(128)
					}
				}
			}
			if r.Address == "module.postgres.azurerm_postgresql_flexible_server.this" {
				expected["storage_mb"] = float64(32768)
				ha, _ := moduleInputs(env, "postgres")["multi_zone"].(bool)
				blocks, _ := r.Values["high_availability"].([]any)
				if (len(blocks) > 0) != ha {
					return fmt.Errorf("plan Postgres HA differs from blueprint")
				}
			}
			for field, value := range expected {
				if err := checkPlanField(r, field, value); err != nil {
					return err
				}
			}
		}
		for _, child := range m.Children {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(*doc.Planned.Root)
}
