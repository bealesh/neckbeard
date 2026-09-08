package estimate

import (
	"strings"
	"testing"
	"time"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/config"
	"gopkg.in/yaml.v3"
)

func reportForTest(t *testing.T, res *Result) []byte {
	t.Helper()
	cfg := &config.Config{
		Tier:      "solo",
		EntryPath: "adopt",
		FinOps:    config.FinOps{MonthlyBudgetAlert: 100},
	}
	bp := &blueprint.Blueprint{
		App: "x", Cloud: "aws", Runtime: "serverless-containers", Tier: "solo", Region: "us-east-1",
		Hash: "sha256:test", Pins: blueprint.Pins{Catalog: "0.1.0", Planner: "test"},
	}
	return Report(res, cfg, bp, time.Unix(0, 0))
}

const skeleton = `version: 0.1
resource_usage:
  module.network.aws_nat_gateway.this[0]:
    monthly_data_processed_gb: 0.0
  module.runtime_serverless.aws_cloudwatch_log_group.this:
    monthly_data_ingested_gb: 0.0
    storage_gb: 0.0
    monthly_data_scanned_gb: 0.0
  module.storage.aws_s3_bucket.data:
    object_tags: 0
    standard:
      storage_gb: 0.0
      monthly_tier_1_requests: 0
  module.postgres.aws_db_instance.this:
    additional_backup_storage_gb: 0.0
`

func testUsage() map[string]float64 {
	return map[string]float64{
		"nat_processed_gb":        80,
		"log_ingest_gb":           30,
		"backup_gb":               50,
		"storage_growth_gb_month": 10,
		"egress_gb":               150, // no aws_lb in skeleton → must report unmapped elsewhere
	}
}

func toF(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return -1
}

func filled(t *testing.T, mult float64) (map[string]any, map[string]bool) {
	t.Helper()
	out, applied, err := FillUsage([]byte(skeleton), testUsage(), mult)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	return doc["resource_usage"].(map[string]any), applied
}

func TestFillUsageMapsKnownKeys(t *testing.T) {
	ru, applied := filled(t, 1)

	nat := ru["module.network.aws_nat_gateway.this[0]"].(map[string]any)
	if got := toF(nat["monthly_data_processed_gb"]); got != 80 {
		t.Errorf("nat processed = %v, want 80", got)
	}
	// Nested keys (S3's storage class blocks) are reached and scaled.
	s3 := ru["module.storage.aws_s3_bucket.data"].(map[string]any)["standard"].(map[string]any)
	if got := toF(s3["storage_gb"]); got != 120 { // 10/month × factor 12
		t.Errorf("s3 storage = %v, want 120", got)
	}
	// Unmapped keys stay zero rather than guessing.
	if got := toF(s3["monthly_tier_1_requests"]); got != 0 {
		t.Errorf("unmapped key should stay 0, got %v", got)
	}
	for _, k := range []string{"nat_processed_gb", "log_ingest_gb", "backup_gb", "storage_growth_gb_month"} {
		if !applied[k] {
			t.Errorf("usage key %s should be marked applied", k)
		}
	}
	if applied["egress_gb"] {
		t.Error("egress_gb has no aws_lb in this skeleton and must not be marked applied")
	}
}

func TestFillUsageScenarioMultiplier(t *testing.T) {
	ru, _ := filled(t, 0) // baseline: everything zeroed
	nat := ru["module.network.aws_nat_gateway.this[0]"].(map[string]any)
	if got := toF(nat["monthly_data_processed_gb"]); got != 0 {
		t.Errorf("baseline nat = %v, want 0", got)
	}
	ru, _ = filled(t, 2)
	nat = ru["module.network.aws_nat_gateway.this[0]"].(map[string]any)
	if got := toF(nat["monthly_data_processed_gb"]); got != 160 {
		t.Errorf("high nat = %v, want 160", got)
	}
}

func TestResourceTypeExtraction(t *testing.T) {
	cases := map[string]string{
		"module.network.aws_nat_gateway.this[0]":         "aws_nat_gateway",
		"module.postgres.google_sql_database_instance.x": "google_sql_database_instance",
		"azurerm_storage_account.data":                   "azurerm_storage_account",
		"module.something.random_pet.x":                  "",
	}
	for addr, want := range cases {
		if got := resourceType(addr); got != want {
			t.Errorf("resourceType(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestReportNamesUnmappedAssumptions(t *testing.T) {
	// The report must never silently drop an assumption (DESIGN §13.2 section 5).
	res := &Result{
		Envs:          []EnvEstimate{{Env: "dev", Costs: map[string]float64{"baseline": 1, "low": 2, "expected": 3, "high": 4}}},
		Usage:         map[string]float64{"egress_gb": 150},
		UsageSource:   map[string]string{"egress_gb": "tier preset (solo)"},
		UnmappedUsage: []string{"egress_gb"},
		Currency:      "USD",
	}
	// minimal config/blueprint stand-ins are fine for the text assertions
	report := string(reportForTest(t, res))
	if !strings.Contains(report, "egress_gb` has no priceable IaC resource") {
		t.Error("report must name unmapped usage assumptions")
	}
	if !strings.Contains(report, "notify; they do not cap") && !strings.Contains(report, "not cap") {
		t.Error("report must state that budget alerts do not cap spending")
	}
}
