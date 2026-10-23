package estimate

import (
	"fmt"
	"github.com/bealesh/neckbeard/core/blueprint"
	"testing"
)

func TestPlanCannotOverrideBillingOrSizingSilently(t *testing.T) {
	env := blueprint.Environment{Name: "dev", Modules: []blueprint.ModuleUsage{{Name: "runtime-serverless", Inputs: []blueprint.Input{{Key: "cpu", Value: 1}, {Key: "memory_gb", Value: 1}, {Key: "min_instances", Value: 0}, {Key: "max_instances", Value: 1}, {Key: "name_prefix", Value: "example-dev"}}}}}
	for _, tt := range []struct {
		cpu, idle, name string
		wantError       bool
	}{{"1", "false", "example-dev-web", false}, {"1000m", "false", "example-dev-web", false}, {"2", "false", "example-dev-web", true}, {"1", "true", "example-dev-web", true}, {"1", "null", "example-dev-web", true}, {"1", "false", "example-prd-web", true}} {
		data := []byte(fmt.Sprintf(`{"planned_values":{"root_module":{"child_modules":[{"resources":[{"address":"module.runtime_serverless.google_cloud_run_v2_service.http[\"web\"]","type":"google_cloud_run_v2_service","values":{"name":%q,"template":[{"scaling":[{"min_instance_count":0,"max_instance_count":1}],"containers":[{"resources":[{"cpu_idle":%s,"limits":{"cpu":%q,"memory":"1024Mi"}}]}]}]}}]}]}}}`, tt.name, tt.idle, tt.cpu))
		if err := validateGCPPlan(data, env, 1); (err != nil) != tt.wantError {
			t.Fatalf("%+v: %v", tt, err)
		}
	}
}
