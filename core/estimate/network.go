package estimate

import (
	"fmt"
	"math"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
)

func isGCPIngressRule(r *pricedResource) bool {
	return r.ResourceType == "google_compute_global_forwarding_rule" && (strings.HasPrefix(r.Name, "module.dns_ingress.") || strings.HasPrefix(r.Name, "neckbeard.gke_ingress"))
}

func correctNetwork(d *breakdown, bp *blueprint.Blueprint, env blueprint.Environment, usage map[string]float64, mult float64, lookup productLookup) ([]string, error) {
	http := 0
	for _, s := range bp.Services {
		if s.Kind == "http" {
			http++
		}
	}
	if bp.Cloud == "gcp" {
		if usage["cross_zone_gb"] > 0 {
			rate, err := lookup(priceQuery{Vendor: "gcp", Region: bp.Region, Service: "Compute Engine", Unit: "gibibyte", Attributes: []priceAttribute{{Key: "description", Value: "Network Inter Zone Data Transfer Out"}}})
			if err != nil {
				return nil, err
			}
			if err := addResource(d, &pricedResource{Name: "neckbeard.cross_zone", ResourceType: "neckbeard_network_transfer", CostComponents: []*costComponent{component("Cross-zone transfer (total billed GiB)", "GiB", rate, usage["cross_zone_gb"]*mult)}}); err != nil {
				return nil, err
			}
		}
		if bp.Runtime == "kubernetes" {
			for i := 0; i < http; i++ {
				if err := addResource(d, &pricedResource{Name: fmt.Sprintf("neckbeard.gke_ingress[%d]", i), ResourceType: "google_compute_global_forwarding_rule"}); err != nil {
					return nil, err
				}
			}
		}
		if bp.Runtime == "kubernetes" {
			return []string{"Controller-created Kubernetes ingress is included in network pricing; GKE manifests currently request HTTP forwarding rules."}, nil
		}
		return nil, nil
	}
	traffic := &pricedResource{Name: "neckbeard.network_transfer", ResourceType: "neckbeard_network_transfer"}
	if bp.Cloud == "aws" {
		rate, err := lookup(priceQuery{Vendor: "aws", Service: "AWSDataTransfer", Family: "Data Transfer", PurchaseOption: "on_demand", Unit: "GB", HighestTier: true, Attributes: []priceAttribute{{Key: "fromRegionCode", Value: bp.Region}, {Key: "transferType", Value: "AWS Outbound"}}})
		if err != nil {
			return nil, err
		}
		traffic.CostComponents = append(traffic.CostComponents, component("Internet transfer (highest list tier)", "GB", rate, usage["egress_gb"]*mult))
		if usage["cross_zone_gb"] > 0 {
			rate, err = lookup(priceQuery{Vendor: "aws", Service: "AWSDataTransfer", Family: "Data Transfer", PurchaseOption: "on_demand", Unit: "GB", Attributes: []priceAttribute{{Key: "fromRegionCode", Value: bp.Region}, {Key: "transferType", Value: "IntraRegion"}, {Key: "usagetype", Regex: "/DataTransfer-Regional-Bytes$/"}}})
			if err != nil {
				return nil, err
			}
			traffic.CostComponents = append(traffic.CostComponents, component("Cross-zone transfer (total billed GB)", "GB", rate, usage["cross_zone_gb"]*mult))
		}
		if http > 0 {
			hourly, err := lookup(priceQuery{Vendor: "aws", Service: "AWSELB", Region: bp.Region, PurchaseOption: "on_demand", Unit: "Hrs", Attributes: []priceAttribute{{Key: "operation", Value: "LoadBalancing:Application"}, {Key: "usagetype", Regex: "/^(?!TS-|Outposts-)(?:[A-Z0-9]+-)?LoadBalancerUsage$/"}}})
			if err != nil {
				return nil, err
			}
			lcu, err := lookup(priceQuery{Vendor: "aws", Service: "AWSELB", Region: bp.Region, PurchaseOption: "on_demand", Unit: "LCU-Hrs", Attributes: []priceAttribute{{Key: "operation", Value: "LoadBalancing:Application"}, {Key: "usagetype", Regex: "/^(?!Outposts-)(?:[A-Z0-9]+-)?LCUUsage$/"}}})
			if err != nil {
				return nil, err
			}
			ip, err := lookup(priceQuery{Vendor: "aws", Service: "AmazonVPC", Region: bp.Region, PurchaseOption: "on_demand", Unit: "Hrs", Attributes: []priceAttribute{{Key: "usagetype", Regex: "/PublicIPv4:InUseAddress$/"}}})
			if err != nil {
				return nil, err
			}
			count := 1
			if bp.Runtime == "kubernetes" {
				count = http
			}
			zones, err := inputNumber(moduleInputs(env, "network"), "zones")
			if err != nil {
				return nil, err
			}
			// 25 new connections/sec per LCU; conservatively one connection/request.
			lcuHours := math.Max((usage["egress_gb"]+usage["ingress_gb"])*mult, usage["requests_per_month"]*float64(http)*mult/90000)
			lb := &pricedResource{Name: "neckbeard.application_ingress", ResourceType: "neckbeard_ingress", CostComponents: []*costComponent{component("Application load balancers", "hours", hourly, float64(count)*hoursPerMonth), component("ALB capacity (bytes or new connections)", "LCU hours", lcu, lcuHours), component("ALB public IPv4 addresses", "address hours", ip, float64(count)*zones*hoursPerMonth)}}
			for _, p := range d.Projects {
				for _, r := range p.Breakdown.Resources {
					if r.ResourceType == "aws_lb" && strings.HasPrefix(r.Name, "module.dns_ingress.") {
						r.CostComponents = nil
						r.Subresources = nil
					}
				}
			}
			if err := addResource(d, lb); err != nil {
				return nil, err
			}
		}
	} else {
		rate, err := lookup(priceQuery{Vendor: "azure", Service: "Bandwidth", Region: bp.Region, PurchaseOption: "Consumption", HighestTier: true, Unit: "1 GB", Attributes: []priceAttribute{{Key: "productName", Value: "Bandwidth - Routing Preference: Microsoft Global network"}, {Key: "meterName", Value: "Standard Data Transfer Out"}}})
		if err != nil {
			return nil, err
		}
		traffic.CostComponents = append(traffic.CostComponents, component("Internet transfer (highest list tier)", "GB", rate, usage["egress_gb"]*mult))
		// Managed ingress is priced separately from compute, including the public IPs.
		if bp.Runtime == "serverless-containers" || bp.Runtime == "kubernetes" {
			hourly, err := azurePrice(lookup, "Global", "Load Balancer", "Standard Included LB Rules and Outbound Rules", "1 Hour")
			if err != nil {
				return nil, err
			}
			data, err := azurePrice(lookup, "Global", "Load Balancer", "Standard Data Processed", "1 GB")
			if err != nil {
				return nil, err
			}
			ip, err := azurePrice(lookup, bp.Region, "Virtual Network", "Standard IPv4 Static Public IP", "1 Hour", priceAttribute{Key: "productName", Value: "IP Addresses"})
			if err != nil {
				return nil, err
			}
			ips := 1.0
			if bp.Runtime == "kubernetes" && http > 0 {
				ips = 2
			}
			if err := addResource(d, &pricedResource{Name: "neckbeard.managed_ingress", ResourceType: "neckbeard_ingress", CostComponents: []*costComponent{component("Standard managed load balancer (first five rules)", "hours", hourly, hoursPerMonth), component("Managed LB data processing", "GB", data, (usage["egress_gb"]+usage["ingress_gb"])*mult), component("Managed public IPv4 addresses", "address hours", ip, ips*hoursPerMonth)}}); err != nil {
				return nil, err
			}
		}
	}
	if err := addResource(d, traffic); err != nil {
		return nil, err
	}
	notes := []string{}
	if bp.Cloud == "azure" {
		notes = append(notes, "Azure inter-availability-zone data transfer is free under the current policy; cross_zone_gb adds no charge.")
	}
	return append(notes, "Network pricing includes internet transfer at the highest list tier without free allowances, public IPv4, and managed/controller ingress. ALB capacity assumes one new connection per HTTP request; cross-zone usage counts total billed GB across both directions."), nil
}
