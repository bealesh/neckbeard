package estimate

import (
	"fmt"
	"math"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
)

const hoursPerMonth = 730.0

// Correct service-level quantities that the pinned adapter cannot infer. Unit
// prices still come from the pricing API in the requested currency.
func correctGCP(d *breakdown, bp *blueprint.Blueprint, env blueprint.Environment, usage map[string]float64, mult float64, lookup rateLookup) ([]string, error) {
	inputs := map[string]map[string]any{}
	for _, m := range env.Modules {
		v := map[string]any{}
		for _, in := range m.Inputs {
			v[in.Key] = in.Value
		}
		inputs[m.Name] = v
	}
	var adjustments []string
	for pi := range d.Projects {
		resources := d.Projects[pi].Breakdown.Resources
		for _, r := range resources {
			if r == nil {
				return nil, fmt.Errorf("null resource in pricing output")
			}
		}
		var rules []*pricedResource
		for _, r := range resources {
			if isGCPIngressRule(r) {
				rules = append(rules, r)
			}
		}
		if len(rules) > 0 {
			minRate, err := lookup("global", "Compute Engine", "/^HTTP Load Balancing: Global Forwarding Rule Minimum Service Charge$/i")
			if err != nil {
				return nil, err
			}
			additionalRate, err := lookup("global", "Compute Engine", "/^HTTP Load Balancing: Global Forwarding Rule Additional Service Charge$/i")
			if err != nil {
				return nil, err
			}
			existing := usage["existing_global_forwarding_rules"]
			charge := func(n float64) float64 {
				if n <= 0 {
					return 0
				}
				return minRate + math.Max(0, n-5)*additionalRate
			}
			rate := charge(existing+float64(len(rules))) - charge(existing)
			for i, r := range rules {
				components := []*costComponent{}
				// The group's charge belongs on one rule; never charge a minimum per port.
				if i == 0 {
					components = append(components, &costComponent{Name: "Global forwarding rules (project bracket)", Unit: "hours", Price: decimalString(rate), MonthlyQuantity: decimalString(hoursPerMonth), MonthlyCost: decimalString(rate * hoursPerMonth)})
				}
				r.CostComponents = components
			}
			if strings.HasPrefix(bp.Region, "us-") {
				internetRate, err := lookup(bp.Region, "Compute Engine", "/^Network Internet Data Transfer Out from Americas to Americas$/i")
				if err != nil {
					return nil, err
				}
				q := usage["egress_gb"] * mult
				rules[0].CostComponents = append(rules[0].CostComponents, &costComponent{Name: "Internet transfer (Americas destination, highest list tier; no free allowance)", Unit: "GiB", Price: decimalString(internetRate), MonthlyQuantity: decimalString(q), MonthlyCost: decimalString(q * internetRate), UsageBased: true})
				adjustments = append(adjustments, "Internet transfer assumes Americas destinations and the highest list-price tier; free allowances and volume discounts are excluded.")
			} else {
				return nil, fmt.Errorf("internet transfer needs a destination-specific price model for region %s", bp.Region)
			}
			adjustments = append(adjustments, fmt.Sprintf("Global forwarding rules priced as one project bracket, after %g other/planned rules in this project.", existing))
			for _, r := range resources {
				if r.Name == "module.dns_ingress.google_compute_global_address.this" {
					r.CostComponents = nil
					r.Subresources = nil
					adjustments = append(adjustments, "Forwarding-rule-attached public IP has no separate IP charge.")
				}
				if r.ResourceType == "google_compute_target_http_proxy" || r.ResourceType == "google_compute_target_https_proxy" {
					if strings.HasPrefix(r.Name, "module.dns_ingress.") {
						r.CostComponents = nil
						r.Subresources = nil
					}
				}
			}
			// Application LB data processing is inbound plus outbound, not duplicated
			// on the HTTP redirect and HTTPS forwarding rules.
			inRate, err := lookup(bp.Region, "Networking", "/^Global External Application Load Balancer Inbound Data Processing for /i")
			if err != nil {
				return nil, err
			}
			outRate, err := lookup(bp.Region, "Networking", "/^Global External Application Load Balancer Outbound Data Processing for /i")
			if err != nil {
				return nil, err
			}
			for _, v := range []struct {
				name           string
				rate, quantity float64
			}{{"Inbound load-balancer data", inRate, usage["ingress_gb"] * mult}, {"Outbound load-balancer data", outRate, usage["egress_gb"] * mult}} {
				rules[0].CostComponents = append(rules[0].CostComponents, &costComponent{Name: v.name, Unit: "GiB", Price: decimalString(v.rate), MonthlyQuantity: decimalString(v.quantity), MonthlyCost: decimalString(v.quantity * v.rate), UsageBased: true})
			}
		}
		for _, r := range resources {
			if r.Name == "module.network.google_service_networking_connection.psa" {
				// The adapter models VPN traffic on a private-service peering connection.
				// The connection itself has no separate usage charge.
				r.CostComponents = nil
				r.Subresources = nil
				adjustments = append(adjustments, "Private-service peering is not a VPN or an internet gateway; no fabricated connection traffic charge.")
			}
			if strings.HasPrefix(r.Name, "module.postgres.google_sql_database_instance.") {
				children := r.Subresources[:0]
				for _, child := range r.Subresources {
					if child.Name != "Replica" {
						children = append(children, child)
					} else {
						adjustments = append(adjustments, r.Name+": empty replica configuration does not create a replica charge; real replica resources remain priced.")
					}
				}
				r.Subresources = children
			}
			if strings.HasPrefix(r.Name, "module.runtime_serverless.") && r.ResourceType == "google_cloud_run_v2_service" {
				runtime := inputs["runtime-serverless"]
				cpu, err := inputNumber(runtime, "cpu")
				if err != nil {
					return nil, err
				}
				memory, err := inputNumber(runtime, "memory_gb")
				if err != nil {
					return nil, err
				}
				floor, err := inputNumber(runtime, "min_instances")
				if err != nil {
					return nil, err
				}
				max, err := inputNumber(runtime, "max_instances")
				if err != nil {
					return nil, err
				}
				hours := math.Max(floor*hoursPerMonth, math.Min(usage["active_instance_hours"]*mult, max*hoursPerMonth))
				cpuRate, err := lookup(bp.Region, "Cloud Run", "/^Services CPU \\(Instance-based billing\\) in "+bp.Region+"$/i")
				if err != nil {
					return nil, err
				}
				memRate, err := lookup(bp.Region, "Cloud Run", "/^Services Memory \\(Instance-based billing\\) in "+bp.Region+"$/i")
				if err != nil {
					return nil, err
				}
				r.CostComponents = nil
				r.Subresources = nil
				for _, v := range []struct {
					name           string
					rate, quantity float64
				}{{"CPU (instance-based billing)", cpuRate, hours * 3600 * cpu}, {"Memory (instance-based billing)", memRate, hours * 3600 * memory}} {
					r.CostComponents = append(r.CostComponents, &costComponent{Name: v.name, Price: decimalString(v.rate), MonthlyQuantity: decimalString(v.quantity), MonthlyCost: decimalString(v.rate * v.quantity), UsageBased: true})
				}
				adjustments = append(adjustments, r.Name+": instance-based compute uses active instance-hours, bounded by the configured minimum and maximum; requests are not separately billed.")
			}
		}
	}
	if len(d.Projects) > 0 {
		rate, err := lookup("global", "Cloud Logging", "/^Log Volume$/i")
		if err != nil {
			return nil, err
		}
		q := usage["log_ingest_gb"] * mult
		d.Projects[0].Breakdown.Resources = append(d.Projects[0].Breakdown.Resources, &pricedResource{Name: "neckbeard.cloud_logging", ResourceType: "neckbeard_logging_usage", CostComponents: []*costComponent{{Name: "Log ingestion (free allowance excluded)", Unit: "GiB", Price: decimalString(rate), MonthlyQuantity: decimalString(q), MonthlyCost: decimalString(q * rate), UsageBased: true}}})
	}
	// Worker pools are absent from the pinned adapter's priceable resources.
	count := 0
	for _, svc := range bp.Services {
		if svc.Kind == "worker" {
			count++
		}
	}
	count = min(count, d.Summary.UnsupportedResourceCounts["google_cloud_run_v2_worker_pool"]+d.Summary.NoPriceResourceCounts["google_cloud_run_v2_worker_pool"])
	if count > 0 && len(d.Projects) == 1 {
		runtime := inputs["runtime-serverless"]
		cpu, err := inputNumber(runtime, "cpu")
		if err != nil {
			return nil, err
		}
		memory, err := inputNumber(runtime, "memory_gb")
		if err != nil {
			return nil, err
		}
		floor, err := inputNumber(runtime, "min_instances")
		if err != nil {
			return nil, err
		}
		cpuRate, err := lookup(bp.Region, "Cloud Run", "/^Worker Pools CPU in "+bp.Region+"$/i")
		if err != nil {
			return nil, err
		}
		memRate, err := lookup(bp.Region, "Cloud Run", "/^Worker Pools Memory in "+bp.Region+"$/i")
		if err != nil {
			return nil, err
		}
		r := &pricedResource{Name: "module.runtime_serverless.worker_pools", ResourceType: "google_cloud_run_v2_worker_pool"}
		seconds := float64(count) * math.Max(1, floor) * hoursPerMonth * 3600
		for _, v := range []struct {
			name           string
			rate, quantity float64
		}{{"Worker pool CPU", cpuRate, seconds * cpu}, {"Worker pool memory", memRate, seconds * memory}} {
			r.CostComponents = append(r.CostComponents, &costComponent{Name: v.name, Price: decimalString(v.rate), MonthlyQuantity: decimalString(v.quantity), MonthlyCost: decimalString(v.rate * v.quantity)})
		}
		d.Projects[0].Breakdown.Resources = append(d.Projects[0].Breakdown.Resources, r)
		modeledInstances(d, "google_cloud_run_v2_worker_pool", count)
		adjustments = append(adjustments, "Worker pools use the catalog's manual instance count and bill continuously, including the idle baseline.")
	}
	return adjustments, d.recalculate()
}

func inputNumber(inputs map[string]any, key string) (float64, error) {
	v, ok := inputs[key]
	if !ok {
		return 0, fmt.Errorf("missing runtime cost input %s", key)
	}
	return number(fmt.Sprint(v), "runtime "+key)
}
