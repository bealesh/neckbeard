package estimate

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/blueprint"
)

func testRates(_, _, description string) (float64, error) {
	switch {
	case strings.Contains(description, "Log Volume"):
		return .5, nil
	case strings.Contains(description, "Worker Pools CPU"):
		return .000011244, nil
	case strings.Contains(description, "Worker Pools Memory"):
		return .000001235, nil
	case strings.Contains(description, "Minimum Service"):
		return .025, nil
	case strings.Contains(description, "Additional Service"):
		return .01, nil
	case strings.Contains(description, "Network Internet Data Transfer Out"):
		return .12, nil
	case strings.Contains(description, "Data Processing"):
		return .008, nil
	case strings.Contains(description, "Services CPU"):
		return .000018, nil
	case strings.Contains(description, "Services Memory"):
		return .000002, nil
	default:
		return 0, fmt.Errorf("unexpected SKU %s", description)
	}
}

func testDocument(rs ...*pricedResource) *breakdown {
	d := &breakdown{TotalMonthlyCost: "0"}
	d.Projects = append(d.Projects, struct {
		Breakdown struct {
			Resources []*pricedResource `json:"resources"`
		} `json:"breakdown"`
	}{})
	d.Projects[0].Breakdown.Resources = rs
	return d
}

func TestGCPForwardingGroupBracketsAndAttachedIP(t *testing.T) {
	for _, tt := range []struct {
		rules             int
		existing, monthly float64
	}{{1, 0, 18.25}, {2, 0, 18.25}, {5, 0, 18.25}, {6, 0, 25.55}, {2, 4, 7.3}, {2, 1, 0}} {
		t.Run(fmt.Sprint(tt.rules, "-", tt.existing), func(t *testing.T) {
			rs := []*pricedResource{{Name: "module.dns_ingress.google_compute_global_address.this", ResourceType: "google_compute_global_address", CostComponents: []*costComponent{{Name: "IP address (unused)", MonthlyCost: decimalString(7.3)}}}}
			for i := 0; i < tt.rules; i++ {
				rs = append(rs, &pricedResource{Name: fmt.Sprintf("module.dns_ingress.rule%d", i), ResourceType: "google_compute_global_forwarding_rule"})
			}
			d := testDocument(rs...)
			_, err := correctGCP(d, &blueprint.Blueprint{Region: "us-central1"}, blueprint.Environment{}, map[string]float64{"existing_global_forwarding_rules": tt.existing}, 1, testRates)
			if err != nil {
				t.Fatal(err)
			}
			n, _ := number(d.TotalMonthlyCost, "total")
			if math.Abs(n-tt.monthly) > 1e-9 {
				t.Fatalf("got %v, want %v", n, tt.monthly)
			}
		})
	}
}

func TestGCPInstanceHoursAndReplica(t *testing.T) {
	for _, tt := range []struct{ hours, mult, min, max, want float64 }{{730, 0, 0, 1, 9.365}, {730, 1, 0, 1, 61.925}, {730, 2, 0, 1, 61.925}, {10, 1, 0, 3, 10.085}, {0, 0, 1, 3, 61.925}} {
		d := testDocument(&pricedResource{Name: "module.postgres.google_sql_database_instance.this", ResourceType: "google_sql_database_instance", CostComponents: []*costComponent{{MonthlyCost: decimalString(9.365)}}, Subresources: []*pricedResource{{Name: "Replica", CostComponents: []*costComponent{{MonthlyCost: decimalString(9.365)}}}}}, &pricedResource{Name: "module.runtime_serverless.http", ResourceType: "google_cloud_run_v2_service"})
		env := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "runtime-serverless", Inputs: []blueprint.Input{{Key: "cpu", Value: "1"}, {Key: "memory_gb", Value: 1}, {Key: "min_instances", Value: tt.min}, {Key: "max_instances", Value: tt.max}}}}}
		_, err := correctGCP(d, &blueprint.Blueprint{Region: "us-central1"}, env, map[string]float64{"active_instance_hours": tt.hours}, tt.mult, testRates)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := number(d.TotalMonthlyCost, "total")
		if math.Abs(n-tt.want) > 1e-9 {
			t.Fatalf("hours %v, multiplier %v: got %v want %v", tt.hours, tt.mult, n, tt.want)
		}
	}
}

func TestGCPWorkerPoolBillsWhileIdle(t *testing.T) {
	d := testDocument()
	d.Summary.UnsupportedResourceCounts = map[string]int{"google_cloud_run_v2_worker_pool": 1}
	env := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "runtime-serverless", Inputs: []blueprint.Input{{Key: "cpu", Value: 1}, {Key: "memory_gb", Value: 1}, {Key: "min_instances", Value: 0}}}}}
	_, err := correctGCP(d, &blueprint.Blueprint{Region: "us-central1", Services: []blueprint.Service{{Name: "worker", Kind: "worker"}}}, env, map[string]float64{}, 0, testRates)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := number(d.TotalMonthlyCost, "total")
	if math.Abs(n-32.794812) > 1e-8 {
		t.Fatalf("worker idle compute: got %v", n)
	}
	if d.Summary.UnsupportedResourceCounts["google_cloud_run_v2_worker_pool"] != 0 {
		t.Fatal("priced worker still marked unsupported")
	}
}

// Replay the upstream output that produced the original wrong Umami estimate.
func TestUmamiPlanRegression(t *testing.T) {
	data, err := os.ReadFile("testdata/umami-plan-breakdown.json")
	if err != nil {
		t.Fatal(err)
	}
	var d breakdown
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	env := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "runtime-serverless", Inputs: []blueprint.Input{{Key: "cpu", Value: 1}, {Key: "memory_gb", Value: 1}, {Key: "min_instances", Value: 0}, {Key: "max_instances", Value: 1}}}}}
	_, err = correctGCP(&d, &blueprint.Blueprint{Region: "us-central1"}, env, map[string]float64{"active_instance_hours": 730, "egress_gb": 20, "ingress_gb": 2, "log_ingest_gb": 5}, 1, testRates)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range d.Projects[0].Breakdown.Resources {
		cost, _ := number(*r.MonthlyCost, "cost")
		if r.ResourceType == "google_cloud_run_v2_service" && math.Abs(cost-52.56) > 1e-8 {
			t.Fatalf("active compute: %v", cost)
		}
		if strings.HasPrefix(r.Name, "module.postgres.") && len(r.Subresources) > 0 {
			t.Fatal("fictitious replica survived")
		}
		if r.ResourceType == "google_compute_global_address" && cost != 0 {
			t.Fatal("attached IP charge survived")
		}
	}
}

func TestPricingCatalogWorkerDoesNotHideCustomWorker(t *testing.T) {
	d := testDocument()
	d.Summary.UnsupportedResourceCounts = map[string]int{"google_cloud_run_v2_worker_pool": 2}
	env := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "runtime-serverless", Inputs: []blueprint.Input{{Key: "cpu", Value: 1}, {Key: "memory_gb", Value: 1}, {Key: "min_instances", Value: 0}}}}}
	bp := &blueprint.Blueprint{Region: "us-central1", Services: []blueprint.Service{{Name: "worker", Kind: "worker"}}}
	if _, err := correctGCP(d, bp, env, map[string]float64{}, 1, testRates); err != nil {
		t.Fatal(err)
	}
	if d.Summary.UnsupportedResourceCounts["google_cloud_run_v2_worker_pool"] != 1 {
		t.Fatal("unsupported custom worker disappeared")
	}
}
