package estimate

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// usageMapping ties an infracost usage key on a resource type to one of our named
// usage assumptions (DESIGN §13.2 — the tier numbers), with an optional factor
// where the semantics need translation. Keys not mapped here stay zero, and
// assumptions that map to nothing are reported as unmodeled — never silently
// dropped either way.
type usageMapping struct {
	OurKey string
	Factor float64
}

var mappings = map[string]map[string]usageMapping{
	// AWS
	"aws_nat_gateway": {
		"monthly_data_processed_gb": {OurKey: "nat_processed_gb", Factor: 1},
	},
	"aws_cloudwatch_log_group": {
		"monthly_data_ingested_gb": {OurKey: "log_ingest_gb", Factor: 1},
		// ~one retention window resident at steady state (30d retention).
		"storage_gb": {OurKey: "log_ingest_gb", Factor: 1},
	},
	"aws_db_instance": {
		"additional_backup_storage_gb": {OurKey: "backup_gb", Factor: 1},
	},
	"aws_s3_bucket": {
		// Steady-state volume after a year of growth.
		"storage_gb": {OurKey: "storage_growth_gb_month", Factor: 12},
	},
	"aws_lb": {
		"processed_bytes_gb": {OurKey: "egress_gb", Factor: 1},
	},
	// GCP
	"google_compute_router_nat": {
		"monthly_data_processed_gb": {OurKey: "nat_processed_gb", Factor: 1},
	},
	"google_cloud_run_v2_service": {
		"monthly_requests": {OurKey: "requests_per_month", Factor: 1},
	},
	"google_storage_bucket": {
		"storage_gb": {OurKey: "storage_growth_gb_month", Factor: 12},
	},
	"google_sql_database_instance": {
		"backup_storage_gb": {OurKey: "backup_gb", Factor: 1},
	},
	"google_compute_global_forwarding_rule": {
		"monthly_ingress_data_gb": {OurKey: "egress_gb", Factor: 1},
	},
	// Azure
	"azurerm_postgresql_flexible_server": {
		"additional_backup_storage_gb": {OurKey: "backup_gb", Factor: 1},
	},
	"azurerm_storage_account": {
		"storage_gb":  {OurKey: "storage_growth_gb_month", Factor: 12},
		"capacity_gb": {OurKey: "storage_growth_gb_month", Factor: 12},
	},
	"azurerm_nat_gateway": {
		"monthly_data_processed_gb": {OurKey: "nat_processed_gb", Factor: 1},
	},
	"azurerm_log_analytics_workspace": {
		"monthly_log_data_ingestion_gb": {OurKey: "log_ingest_gb", Factor: 1},
	},
}

// FillUsage takes the skeleton produced by --sync-usage-file (every resource
// address with its usage keys zeroed — the ground truth for which keys exist) and
// fills the keys we can honestly map, scaled by the scenario multiplier. Returns
// the filled YAML and the set of our usage keys that actually attached somewhere.
func FillUsage(skeleton []byte, usage map[string]float64, multiplier float64) ([]byte, map[string]bool, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(skeleton, &doc); err != nil {
		return nil, nil, fmt.Errorf("parsing usage skeleton: %w", err)
	}
	applied := map[string]bool{}
	if ru, ok := doc["resource_usage"].(map[string]any); ok {
		for addr, node := range ru {
			entry, ok := mappings[resourceType(addr)]
			if !ok {
				continue
			}
			fillNode(node, entry, usage, multiplier, applied)
		}
	}
	out, err := yaml.Marshal(doc)
	return out, applied, err
}

func fillNode(node any, entry map[string]usageMapping, usage map[string]float64, multiplier float64, applied map[string]bool) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	for k, v := range m {
		if child, ok := v.(map[string]any); ok {
			fillNode(child, entry, usage, multiplier, applied)
			continue
		}
		if um, ok := entry[k]; ok {
			if base, ok := usage[um.OurKey]; ok {
				m[k] = base * um.Factor * multiplier
				applied[um.OurKey] = true
			}
		}
	}
}

// resourceType extracts the provider resource type from an infracost usage
// address like module.network.aws_nat_gateway.this[0].
func resourceType(addr string) string {
	for _, seg := range strings.Split(addr, ".") {
		if strings.HasPrefix(seg, "aws_") || strings.HasPrefix(seg, "google_") || strings.HasPrefix(seg, "azurerm_") {
			return seg
		}
	}
	return ""
}
