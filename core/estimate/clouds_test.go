package estimate

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/blueprint"
)

func cloudRates(q priceQuery) (float64, error) {
	for _, a := range q.Attributes {
		switch {
		case a.Value == "Balanced PD Capacity":
			return .1, nil
		case strings.Contains(a.Regex, "Fargate-vCPU"):
			return .04048, nil
		case strings.Contains(a.Regex, "Fargate-GB"):
			return .004445, nil
		case a.Value == "Standard vCPU Active Usage":
			return .000024, nil
		case a.Value == "Standard Memory Active Usage" || a.Value == "Standard Memory Idle Usage" || a.Value == "Standard vCPU Idle Usage":
			return .000003, nil
		case a.Value == "Standard Requests":
			return .4, nil
		case strings.Contains(a.Value, "D2as v5 Spot"):
			return .018155, nil
		}
	}
	return 0, fmt.Errorf("unhandled price query %+v", q)
}

func serverlessEnv(floor, max float64) blueprint.Environment {
	return blueprint.Environment{Name: "dev", Modules: []blueprint.ModuleUsage{{Name: "runtime-serverless", Inputs: []blueprint.Input{{Key: "cpu", Value: .5}, {Key: "memory_gb", Value: 1}, {Key: "min_instances", Value: floor}, {Key: "max_instances", Value: max}}}}}
}

func TestServerlessProviderBilling(t *testing.T) {
	for _, tt := range []struct {
		cloud           string
		mult, min, want float64
	}{
		{"aws", 0, 0, 36.0401}, {"aws", 1, 0, 36.0524425}, {"aws", 2, 0, 54.084835},
		{"azure", 0, 0, 39.42}, {"azure", 1, 0, 79.667}, {"azure", 0, 1, 51.246},
	} {
		t.Run(fmt.Sprint(tt), func(t *testing.T) {
			bp := &blueprint.Blueprint{Cloud: tt.cloud, Region: "us-east-1", Runtime: "serverless-containers", Services: []blueprint.Service{{Name: "web", Kind: "http"}, {Name: "worker", Kind: "worker"}, {Name: "daily", Kind: "cron"}}}
			d := testDocument(&pricedResource{Name: "module.runtime_serverless.aws_ecs_service.web", ResourceType: "aws_ecs_service", CostComponents: []*costComponent{component("old compute", "hours", 1, 5000)}})
			_, err := correctServerless(d, bp, serverlessEnv(tt.min, 3), map[string]float64{"active_instance_hours": 730, "requests_per_month": 2e6, "job_executions_per_month": 30, "job_duration_minutes": 1}, tt.mult, cloudRates)
			if err != nil {
				t.Fatal(err)
			}
			if err := d.recalculate(); err != nil {
				t.Fatal(err)
			}
			got, _ := number(d.TotalMonthlyCost, "total")
			if math.Abs(got-tt.want) > 1e-7 {
				t.Fatalf("got %.9f want %.9f", got, tt.want)
			}
		})
	}
}

func TestAzureJobsHaveNoFargateMinuteMinimum(t *testing.T) {
	bp := &blueprint.Blueprint{Cloud: "azure", Runtime: "serverless-containers", Services: []blueprint.Service{{Name: "short", Kind: "cron"}}}
	d := testDocument()
	_, err := correctServerless(d, bp, serverlessEnv(0, 3), map[string]float64{"job_executions_per_month": 1, "job_duration_minutes": .1}, 1, cloudRates)
	if err != nil {
		t.Fatal(err)
	}
	_ = d.recalculate()
	got, _ := number(d.TotalMonthlyCost, "total")
	if math.Abs(got-.00009) > 1e-10 {
		t.Fatalf("six-second Azure job: %v", got)
	}
}

func TestGKENodeHoursAreTotalAcrossZones(t *testing.T) {
	for _, tt := range []struct{ extra, mult, want float64 }{{0, 0, 97.67738}, {730, 1, 122.35476}, {7300, 2, 122.35476}} {
		pool := &pricedResource{Name: "module.runtime_k8s.google_container_node_pool.default", ResourceType: "google_container_node_pool", CostComponents: []*costComponent{component("Instance usage", "hours", .020106, 9*730), component("Standard provisioned storage", "GB", .04, 900)}}
		d := testDocument(pool, &pricedResource{Name: "module.runtime_k8s.google_container_cluster.this", ResourceType: "google_container_cluster", CostComponents: []*costComponent{component("Control plane", "hours", .1, 730)}})
		env := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "runtime-k8s", Inputs: []blueprint.Input{{Key: "min_nodes", Value: 1}, {Key: "max_nodes", Value: 2}}}}}
		_, err := correctNodes(d, &blueprint.Blueprint{Cloud: "gcp", Runtime: "kubernetes"}, env, map[string]float64{"additional_node_hours": tt.extra}, tt.mult, cloudRates)
		if err != nil {
			t.Fatal(err)
		}
		_ = d.recalculate()
		got, _ := number(d.TotalMonthlyCost, "total")
		if math.Abs(got-tt.want) > 1e-7 {
			t.Fatalf("got %v want %v", got, tt.want)
		}
	}
}

func TestAKSFixedSystemPoolAndSpotUserPool(t *testing.T) {
	system := &pricedResource{Name: "module.runtime_k8s.azurerm_kubernetes_cluster.this", ResourceType: "azurerm_kubernetes_cluster", CostComponents: []*costComponent{component("Control plane", "hours", .1, 730)}, Subresources: []*pricedResource{{Name: "default_node_pool", CostComponents: []*costComponent{component("Instance usage", "hours", .086, 730)}}}}
	spot := &pricedResource{Name: "module.runtime_k8s.azurerm_kubernetes_cluster_node_pool.spot[0]", ResourceType: "azurerm_kubernetes_cluster_node_pool", CostComponents: []*costComponent{component("Instance usage", "hours", .086, 730)}, Subresources: []*pricedResource{{Name: "disk", CostComponents: []*costComponent{component("OS disk", "months", 19.71, 1)}}}}
	d := testDocument(system, spot)
	env := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "runtime-k8s", Inputs: []blueprint.Input{{Key: "min_nodes", Value: 2}, {Key: "max_nodes", Value: 4}, {Key: "spot", Value: true}, {Key: "node_shape", Value: "small"}}}}}
	_, err := correctNodes(d, &blueprint.Blueprint{Cloud: "azure", Region: "eastus", Runtime: "kubernetes"}, env, map[string]float64{}, 0, cloudRates)
	if err != nil {
		t.Fatal(err)
	}
	_ = d.recalculate()
	got, _ := number(d.TotalMonthlyCost, "total")
	// The system node stays on demand; two Spot nodes also retain their OS disks.
	want := 73 + 62.78 + 2*(13.25315+19.71)
	if math.Abs(got-want) > 1e-7 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAzureHAAndInheritedReplicaStorage(t *testing.T) {
	for _, present := range []bool{false, true} {
		primary := &pricedResource{Name: "module.postgres.azurerm_postgresql_flexible_server.this", ResourceType: "azurerm_postgresql_flexible_server", CostComponents: []*costComponent{component("Compute (primary)", "hours", .1, 730), {Name: "Storage", Price: decimalString(0), PriceNotFound: true}, component("Additional backup storage", "GB", .095, 10)}}
		d := testDocument(primary)
		missing := 2
		if present {
			d.Projects[0].Breakdown.Resources = append(d.Projects[0].Breakdown.Resources, &pricedResource{Name: "module.postgres.azurerm_postgresql_flexible_server.replica[0]", ResourceType: "azurerm_postgresql_flexible_server"})
			missing = 1
		}
		d.Summary.UnsupportedResourceCounts = map[string]int{"azurerm_postgresql_flexible_server": missing}
		env := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "postgres", Inputs: []blueprint.Input{{Key: "read_replicas", Value: 1}, {Key: "multi_zone", Value: true}}}}}
		_, err := correctAzureDatabase(d, &blueprint.Blueprint{Cloud: "azure", Region: "eastus"}, env, func(q priceQuery) (float64, error) {
			if q.Unit != "1 GB/Month" {
				t.Fatalf("storage unit: %s", q.Unit)
			}
			return .115, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = d.recalculate()
		got, _ := number(d.TotalMonthlyCost, "total")
		if math.Abs(got-230.99) > 1e-8 {
			t.Fatalf("HA + one replica: %v", got)
		}
		if d.Summary.UnsupportedResourceCounts["azurerm_postgresql_flexible_server"] != 1 {
			t.Fatal("pricing the catalog replica hid an unsupported user database")
		}
	}
}

func TestMissingClusterCannotProduceAnEstimate(t *testing.T) {
	d := testDocument()
	env := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "runtime-k8s", Inputs: []blueprint.Input{{Key: "min_nodes", Value: 1}, {Key: "max_nodes", Value: 2}}}}}
	if _, err := correctNodes(d, &blueprint.Blueprint{Cloud: "aws", Runtime: "kubernetes"}, env, map[string]float64{}, 1, cloudRates); err == nil {
		t.Fatal("missing cluster priced as zero")
	}
}
