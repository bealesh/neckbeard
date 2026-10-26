package estimate

import (
	"encoding/json"
	"github.com/bealesh/neckbeard/core/blueprint"
	"testing"
)

func TestPlansMustMatchPricedComputeAndPoolSettings(t *testing.T) {
	env := serverlessEnv(0, 3)
	aws := &blueprint.Blueprint{Cloud: "aws", Runtime: "serverless-containers"}
	resource := costPlanResource{Address: "module.runtime_serverless.aws_ecs_task_definition.service[\"web\"]", Type: "aws_ecs_task_definition", Values: map[string]any{"cpu": "512", "memory": "1024"}}
	data := func(r costPlanResource) []byte {
		out, _ := json.Marshal(map[string]any{"planned_values": map[string]any{"root_module": map[string]any{"resources": []costPlanResource{r}}}})
		return out
	}
	if err := validateCostPlan(data(resource), aws, env); err != nil {
		t.Fatal(err)
	}
	resource.Values["cpu"] = "1024"
	if err := validateCostPlan(data(resource), aws, env); err == nil {
		t.Fatal("different task CPU accepted")
	}
	nodes := blueprint.Environment{Modules: []blueprint.ModuleUsage{{Name: "runtime-k8s", Inputs: []blueprint.Input{{Key: "min_nodes", Value: 1}, {Key: "max_nodes", Value: 2}, {Key: "spot", Value: true}, {Key: "node_shape", Value: "small"}}}}}
	resource = costPlanResource{Address: "module.runtime_k8s.azurerm_kubernetes_cluster_node_pool.spot[0]", Type: "azurerm_kubernetes_cluster_node_pool", Values: map[string]any{"vm_size": "Standard_D2as_v5", "min_count": 1, "max_count": 2, "priority": "Spot"}}
	azure := &blueprint.Blueprint{Cloud: "azure", Runtime: "kubernetes"}
	if err := validateCostPlan(data(resource), azure, nodes); err != nil {
		t.Fatal(err)
	}
	resource.Values["priority"] = "Regular"
	if err := validateCostPlan(data(resource), azure, nodes); err == nil {
		t.Fatal("regular VM priced as Spot")
	}
	resource.Values["priority"] = "Spot"
	resource.Values["max_count"] = 5
	if err := validateCostPlan(data(resource), azure, nodes); err == nil {
		t.Fatal("different autoscaling maximum accepted")
	}
}
