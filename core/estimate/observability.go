package estimate

import "github.com/bealesh/neckbeard/core/blueprint"

func correctObservability(d *breakdown, bp *blueprint.Blueprint, usage map[string]float64, mult float64, lookup productLookup) ([]string, error) {
	if bp.Cloud != "aws" {
		return nil, nil
	}
	lookupCW := func(suffix, unit string, tiers bool) (float64, error) {
		return lookup(priceQuery{Vendor: "aws", Service: "AmazonCloudWatch", Region: bp.Region, PurchaseOption: "on_demand", HighestTier: tiers, Unit: unit, Attributes: []priceAttribute{{Key: "usagetype", Regex: "/(^|-)" + suffix + "$/"}}})
	}
	r := &pricedResource{Name: "neckbeard.cloudwatch", ResourceType: "neckbeard_observability"}
	if bp.Runtime == "kubernetes" {
		ingest, err := lookupCW("DataProcessing-Bytes", "GB", false)
		if err != nil {
			return nil, err
		}
		storage, err := lookupCW("TimedStorage-ByteHrs", "GB-Mo", false)
		if err != nil {
			return nil, err
		}
		query, err := lookupCW("DataScanned-Bytes", "GB", false)
		if err != nil {
			return nil, err
		}
		r.CostComponents = []*costComponent{component("EKS audit/control-plane log ingestion", "GB", ingest, usage["log_ingest_gb"]*mult), component("EKS logs (30-day retained volume assumption)", "GB months", storage, usage["log_ingest_gb"]*mult), component("Logs Insights queries", "GB", query, usage["log_query_gb"]*mult)}
	} else {
		rate, err := lookupCW("CW:MetricMonitorUsage", "Metrics", true)
		if err != nil {
			return nil, err
		}
		r.CostComponents = []*costComponent{component("Container Insights custom metric series", "metrics", rate, usage["custom_metrics"])}
	}
	if err := addResource(d, r); err != nil {
		return nil, err
	}
	return []string{"EKS audit/control-plane logs are priced even though AWS creates the log group outside Terraform. ECS Container Insights uses the explicit custom_metrics count; metric series remain billable at idle."}, nil
}
