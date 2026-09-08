package estimate

import (
	"gopkg.in/yaml.v3"
)

// mapEntry ties one infracost usage key on a resource type to one of our named
// usage assumptions (DESIGN §13.2 — the tier numbers), with a factor where the
// semantics need translation. The usage file is generated with
// resource_type_default_usage, so an assumption applies to every resource of the
// type — exactly the tier-assumption semantic. Keys not mapped stay unset (zero),
// and assumptions whose types are absent from an env are reported as unmodeled —
// never silently dropped either way.
//
// Key names verified against infracost 0.10.45 sync output for AWS
// (2026-09-08); GCP/Azure names follow infracost's documented usage schemas and
// harmlessly no-op if a name drifts (the type simply never matches).
type mapEntry struct {
	Type   string
	Path   []string // nested path within the type's usage block
	OurKey string
	Factor float64
}

var usageMap = []mapEntry{
	// AWS — verified against real sync output.
	{"aws_nat_gateway", []string{"monthly_data_processed_gb"}, "nat_processed_gb", 1},
	// The no-NAT strategy routes the same traffic through VPC endpoints instead.
	{"aws_vpc_endpoint", []string{"monthly_data_processed_gb"}, "nat_processed_gb", 1},
	{"aws_cloudwatch_log_group", []string{"monthly_data_ingested_gb"}, "log_ingest_gb", 1},
	// ~one 30-day retention window resident at steady state.
	{"aws_cloudwatch_log_group", []string{"storage_gb"}, "log_ingest_gb", 1},
	{"aws_db_instance", []string{"additional_backup_storage_gb"}, "backup_gb", 1},
	// Steady-state volume after a year of growth.
	{"aws_s3_bucket", []string{"standard", "storage_gb"}, "storage_growth_gb_month", 12},
	{"aws_lb", []string{"processed_bytes_gb"}, "egress_gb", 1},
	{"aws_ecr_repository", []string{"storage_gb"}, "registry_storage_gb_fixed", 1},

	// GCP
	{"google_compute_router_nat", []string{"monthly_data_processed_gb"}, "nat_processed_gb", 1},
	{"google_cloud_run_v2_service", []string{"monthly_requests"}, "requests_per_month", 1},
	{"google_storage_bucket", []string{"storage_gb"}, "storage_growth_gb_month", 12},
	{"google_sql_database_instance", []string{"backup_storage_gb"}, "backup_gb", 1},

	// Azure
	{"azurerm_postgresql_flexible_server", []string{"additional_backup_storage_gb"}, "backup_gb", 1},
	{"azurerm_storage_account", []string{"capacity_gb"}, "storage_growth_gb_month", 12},
	{"azurerm_nat_gateway", []string{"monthly_data_processed_gb"}, "nat_processed_gb", 1},
	{"azurerm_log_analytics_workspace", []string{"monthly_log_data_ingestion_gb"}, "log_ingest_gb", 1},
}

// fixedAssumptions are small constants that are not tier dimensions but keep
// lines from silently reading as free.
var fixedAssumptions = map[string]float64{
	"registry_storage_gb_fixed": 10, // a handful of image versions resident
}

// BuildUsageFile emits an infracost usage file using resource_type_default_usage,
// scaled by the scenario multiplier.
func BuildUsageFile(usage map[string]float64, multiplier float64) ([]byte, error) {
	defaults := map[string]any{}
	for _, e := range usageMap {
		base, ok := usage[e.OurKey]
		if !ok {
			if base, ok = fixedAssumptions[e.OurKey]; !ok {
				continue
			}
		}
		node, _ := defaults[e.Type].(map[string]any)
		if node == nil {
			node = map[string]any{}
			defaults[e.Type] = node
		}
		cur := node
		for _, seg := range e.Path[:len(e.Path)-1] {
			next, _ := cur[seg].(map[string]any)
			if next == nil {
				next = map[string]any{}
				cur[seg] = next
			}
			cur = next
		}
		cur[e.Path[len(e.Path)-1]] = base * e.Factor * multiplier
	}
	return yaml.Marshal(map[string]any{
		"version":                     "0.1",
		"resource_type_default_usage": defaults,
	})
}

// AppliedUsage reports which of our usage assumptions actually attached to a
// resource type present in the environment.
func AppliedUsage(resourceTypes map[string]bool) map[string]bool {
	applied := map[string]bool{}
	for _, e := range usageMap {
		if resourceTypes[e.Type] && e.OurKey != "" {
			if _, fixed := fixedAssumptions[e.OurKey]; !fixed {
				applied[e.OurKey] = true
			}
		}
	}
	return applied
}
