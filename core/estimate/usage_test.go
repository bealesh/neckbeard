package estimate

import (
	"strings"
	"testing"
	"time"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/config"
	"gopkg.in/yaml.v3"
)

func testUsage() map[string]float64 {
	return map[string]float64{
		"nat_processed_gb":        80,
		"log_ingest_gb":           30,
		"backup_gb":               50,
		"storage_growth_gb_month": 10,
		"egress_gb":               150,
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

func defaultsFor(t *testing.T, mult float64) map[string]any {
	t.Helper()
	out, err := BuildUsageFile(testUsage(), mult)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	d, ok := doc["resource_type_default_usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage file missing resource_type_default_usage: %s", out)
	}
	if doc["version"] != "0.1" {
		t.Errorf("usage file version = %v, want 0.1", doc["version"])
	}
	return d
}

func TestBuildUsageFileMapsAssumptions(t *testing.T) {
	d := defaultsFor(t, 1)

	nat := d["aws_nat_gateway"].(map[string]any)
	if got := toF(nat["monthly_data_processed_gb"]); got != 80 {
		t.Errorf("nat processed = %v, want 80", got)
	}
	// The no-NAT strategy's VPC endpoints carry the same assumption.
	vpce := d["aws_vpc_endpoint"].(map[string]any)
	if got := toF(vpce["monthly_data_processed_gb"]); got != 80 {
		t.Errorf("vpc endpoint processed = %v, want 80", got)
	}
	// Nested paths (S3 storage-class blocks) and factors.
	s3 := d["aws_s3_bucket"].(map[string]any)["standard"].(map[string]any)
	if got := toF(s3["storage_gb"]); got != 120 { // 10/month × factor 12
		t.Errorf("s3 storage = %v, want 120", got)
	}
	if got := toF(d["aws_lb"].(map[string]any)["processed_bytes_gb"]); got != 150 {
		t.Errorf("alb processed bytes = %v, want 150", got)
	}
}

func TestBuildUsageFileScenarioMultiplier(t *testing.T) {
	d := defaultsFor(t, 0) // baseline zeroes everything
	nat := d["aws_nat_gateway"].(map[string]any)
	if got := toF(nat["monthly_data_processed_gb"]); got != 0 {
		t.Errorf("baseline nat = %v, want 0", got)
	}
	d = defaultsFor(t, 2)
	nat = d["aws_nat_gateway"].(map[string]any)
	if got := toF(nat["monthly_data_processed_gb"]); got != 160 {
		t.Errorf("high nat = %v, want 160", got)
	}
}

func TestAppliedUsageTracksPresentTypes(t *testing.T) {
	applied := AppliedUsage(map[string]bool{
		"aws_vpc_endpoint":         true,
		"aws_cloudwatch_log_group": true,
	})
	if !applied["nat_processed_gb"] || !applied["log_ingest_gb"] {
		t.Errorf("expected nat + log assumptions applied, got %v", applied)
	}
	if applied["egress_gb"] {
		t.Error("egress_gb should not apply without an aws_lb")
	}
	if applied["registry_storage_gb_fixed"] {
		t.Error("fixed assumptions are not user-facing usage keys")
	}
}

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

func TestReportNamesUnmappedAssumptions(t *testing.T) {
	// The report must never silently drop an assumption (DESIGN §13.2 section 5).
	res := &Result{
		Envs:          []EnvEstimate{{Env: "dev", Costs: map[string]float64{"baseline": 1, "low": 2, "expected": 3, "high": 4}}},
		Usage:         map[string]float64{"egress_gb": 150},
		UsageSource:   map[string]string{"egress_gb": "tier preset (solo)"},
		UnmappedUsage: []string{"egress_gb"},
		Currency:      "USD",
	}
	report := string(reportForTest(t, res))
	if !strings.Contains(report, "egress_gb` has no priceable IaC resource") {
		t.Error("report must name unmapped usage assumptions")
	}
	if !strings.Contains(report, "not cap") {
		t.Error("report must state that budget alerts do not cap spending")
	}
}
