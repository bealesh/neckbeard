package estimate

import (
	"fmt"
	"math"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
)

func moduleInputs(env blueprint.Environment, name string) map[string]any {
	values := map[string]any{}
	for _, m := range env.Modules {
		if m.Name == name {
			for _, in := range m.Inputs {
				values[in.Key] = in.Value
			}
		}
	}
	return values
}

func component(name, unit string, rate, quantity float64) *costComponent {
	return &costComponent{Name: name, Unit: unit, Price: decimalString(rate), MonthlyQuantity: decimalString(quantity), MonthlyCost: decimalString(rate * quantity)}
}

func addResource(d *breakdown, r *pricedResource) error {
	if len(d.Projects) != 1 {
		return fmt.Errorf("cost corrections require exactly one project per environment")
	}
	d.Projects[0].Breakdown.Resources = append(d.Projects[0].Breakdown.Resources, r)
	return nil
}

func azurePrice(lookup productLookup, region, service, meter, unit string, attrs ...priceAttribute) (float64, error) {
	attrs = append(attrs, priceAttribute{Key: "meterName", Value: meter})
	return lookup(priceQuery{Vendor: "azure", Region: region, Service: service, Attributes: attrs, PurchaseOption: "Consumption", Unit: unit})
}

func fargatePrices(lookup productLookup, region string) (float64, float64, error) {
	cpu, err := lookup(priceQuery{Vendor: "aws", Region: region, Service: "AmazonECS", PurchaseOption: "on_demand", Unit: "hours", Attributes: []priceAttribute{{Key: "usagetype", Regex: "/Fargate-vCPU-Hours:perCPU$/"}}})
	if err != nil {
		return 0, 0, err
	}
	memory, err := lookup(priceQuery{Vendor: "aws", Region: region, Service: "AmazonECS", PurchaseOption: "on_demand", Unit: "hours", Attributes: []priceAttribute{{Key: "usagetype", Regex: "/Fargate-GB-Hours$/"}}})
	return cpu, memory, err
}

func correctServerless(d *breakdown, bp *blueprint.Blueprint, env blueprint.Environment, usage map[string]float64, mult float64, lookup productLookup) ([]string, error) {
	if bp.Runtime != "serverless-containers" || bp.Cloud == "gcp" {
		return nil, nil
	}
	inputs := moduleInputs(env, "runtime-serverless")
	cpu, err := inputNumber(inputs, "cpu")
	if err != nil {
		return nil, err
	}
	memory, err := inputNumber(inputs, "memory_gb")
	if err != nil {
		return nil, err
	}
	floor, err := inputNumber(inputs, "min_instances")
	if err != nil {
		return nil, err
	}
	max, err := inputNumber(inputs, "max_instances")
	if err != nil {
		return nil, err
	}
	var cpuRate, memoryRate, idleCPU, idleMemory, requestsRate float64
	if bp.Cloud == "aws" {
		cpuRate, memoryRate, err = fargatePrices(lookup, bp.Region)
	} else {
		attrs := []priceAttribute{{Key: "skuName", Value: "Standard"}}
		cpuRate, err = azurePrice(lookup, bp.Region, "Azure Container Apps", "Standard vCPU Active Usage", "1 Second", attrs...)
		if err != nil {
			return nil, err
		}
		memoryRate, err = azurePrice(lookup, bp.Region, "Azure Container Apps", "Standard Memory Active Usage", "1 GiB Second", attrs...)
		if err != nil {
			return nil, err
		}
		idleCPU, err = azurePrice(lookup, bp.Region, "Azure Container Apps", "Standard vCPU Idle Usage", "1 Second", attrs...)
		if err != nil {
			return nil, err
		}
		idleMemory, err = azurePrice(lookup, bp.Region, "Azure Container Apps", "Standard Memory Idle Usage", "1 GiB Second", attrs...)
		if err != nil {
			return nil, err
		}
		requestsRate, err = azurePrice(lookup, bp.Region, "Azure Container Apps", "Standard Requests", "1M", attrs...)
	}
	if err != nil {
		return nil, err
	}
	for _, svc := range bp.Services {
		name := "neckbeard.runtime[" + fmt.Sprintf("%q", svc.Name) + "]"
		r := &pricedResource{Name: name, ResourceType: "neckbeard_serverless_compute"}
		if svc.Kind == "cron" {
			duration := math.Max(1, math.Ceil(usage["job_duration_minutes"]*60))
			if bp.Cloud == "aws" {
				duration = math.Max(60, duration)
			}
			seconds := usage["job_executions_per_month"] * mult * duration
			if bp.Cloud == "aws" {
				seconds /= 3600
			}
			r.CostComponents = []*costComponent{component("Scheduled CPU", "CPU time", cpuRate, seconds*cpu), component("Scheduled memory", "GiB time", memoryRate, seconds*memory)}
		} else {
			minimum := floor
			if bp.Cloud == "aws" || svc.Kind == "worker" {
				minimum = math.Max(1, minimum)
			}
			hours := math.Max(minimum*hoursPerMonth, math.Min(usage["active_instance_hours"]*mult, max*hoursPerMonth))
			if svc.Kind == "worker" {
				hours = minimum * hoursPerMonth
			}
			if bp.Cloud == "aws" {
				r.CostComponents = []*costComponent{component("Fargate CPU", "vCPU hours", cpuRate, hours*cpu), component("Fargate memory", "GiB hours", memoryRate, hours*memory)}
			} else {
				active := math.Min(hours, usage["active_instance_hours"]*mult)
				if svc.Kind == "worker" {
					active = hours
				}
				idle := math.Max(0, hours-active)
				r.CostComponents = []*costComponent{component("Active CPU", "vCPU seconds", cpuRate, active*3600*cpu), component("Active memory", "GiB seconds", memoryRate, active*3600*memory), component("Idle CPU", "vCPU seconds", idleCPU, idle*3600*cpu), component("Idle memory", "GiB seconds", idleMemory, idle*3600*memory)}
				if svc.Kind == "http" {
					r.CostComponents = append(r.CostComponents, component("HTTP requests", "million requests", requestsRate, usage["requests_per_month"]*mult/1e6))
				}
			}
		}
		if err := addResource(d, r); err != nil {
			return nil, err
		}
	}
	// Replace the adapter's ECS compute once; task definitions are control-plane
	// objects and do not price scheduled executions. ACA is unsupported upstream.
	for _, p := range d.Projects {
		for _, r := range p.Breakdown.Resources {
			if r.ResourceType == "aws_ecs_service" && strings.HasPrefix(r.Name, "module.runtime_serverless.") {
				r.CostComponents = nil
				r.Subresources = nil
			}
		}
	}
	if bp.Cloud == "azure" {
		apps, jobs := 0, 0
		for _, svc := range bp.Services {
			if svc.Kind == "cron" {
				jobs++
			} else {
				apps++
			}
		}
		modeledInstances(d, "azurerm_container_app", apps)
		modeledInstances(d, "azurerm_container_app_job", jobs)
		modeledInstances(d, "azurerm_container_app_environment", 1)
	}
	return []string{"Runtime compute uses configured CPU/memory, active instance-hours and instance bounds. Workers run continuously; scheduled tasks use explicit execution count and duration. ACA idle replicas use idle rates, and HTTP requests are priced separately. Free allowances are excluded."}, nil
}

func setQuantity(c *costComponent, q float64) error {
	if c.Price == nil || c.PriceNotFound {
		return fmt.Errorf("missing price for %s", c.Name)
	}
	rate, err := number(*c.Price, "unit price")
	if err != nil {
		return err
	}
	c.MonthlyQuantity = decimalString(q)
	c.MonthlyCost = decimalString(q * rate)
	return nil
}

func scaleResource(r *pricedResource, ratio float64) error {
	for _, c := range r.CostComponents {
		if c.MonthlyQuantity == nil || c.MonthlyCost == nil {
			return fmt.Errorf("unresolved node component %s", c.Name)
		}
		q, err := number(*c.MonthlyQuantity, "node quantity")
		if err != nil {
			return err
		}
		cost, err := number(*c.MonthlyCost, "node cost")
		if err != nil {
			return err
		}
		c.MonthlyQuantity = decimalString(q * ratio)
		c.MonthlyCost = decimalString(cost * ratio)
	}
	for _, s := range r.Subresources {
		if err := scaleResource(s, ratio); err != nil {
			return err
		}
	}
	return nil
}

func instanceHours(r *pricedResource) (float64, error) {
	for _, c := range r.CostComponents {
		if strings.HasPrefix(c.Name, "Instance usage") && c.MonthlyQuantity != nil {
			return number(*c.MonthlyQuantity, "node hours")
		}
	}
	return 0, fmt.Errorf("missing instance-hours for node pool %s", r.Name)
}

func correctNodes(d *breakdown, bp *blueprint.Blueprint, env blueprint.Environment, usage map[string]float64, mult float64, lookup productLookup) ([]string, error) {
	if bp.Runtime != "kubernetes" {
		return nil, nil
	}
	inputs := moduleInputs(env, "runtime-k8s")
	min, err := inputNumber(inputs, "min_nodes")
	if err != nil {
		return nil, err
	}
	max, err := inputNumber(inputs, "max_nodes")
	if err != nil {
		return nil, err
	}
	hours := min*hoursPerMonth + math.Min(usage["additional_node_hours"]*mult, math.Max(0, max-min)*hoursPerMonth)
	spot, _ := inputs["spot"].(bool)
	pools, clusters := 0, 0
	for _, p := range d.Projects {
		for _, r := range p.Breakdown.Resources {
			if !strings.HasPrefix(r.Name, "module.runtime_k8s.") {
				continue
			}
			if r.ResourceType == "aws_eks_cluster" || r.ResourceType == "google_container_cluster" {
				clusters++
			}
			pool := r
			switch r.ResourceType {
			case "aws_eks_node_group", "google_container_node_pool", "azurerm_kubernetes_cluster_node_pool":
			case "azurerm_kubernetes_cluster":
				clusters++
				if spot {
					continue
				}
				pool = nil
				for _, s := range r.Subresources {
					if s.Name == "default_node_pool" {
						pool = s
						break
					}
				}
				if pool == nil {
					return nil, fmt.Errorf("missing AKS default node pool")
				}
			default:
				continue
			}
			pools++
			old, err := instanceHours(pool)
			if err != nil {
				return nil, err
			}
			if old <= 0 {
				return nil, fmt.Errorf("node pool has no priced instance hours")
			}
			if r.ResourceType == "azurerm_kubernetes_cluster_node_pool" && spot {
				shape := fmt.Sprint(inputs["node_shape"])
				sku := "Standard_D2as_v5"
				meter := "D2as v5 Spot"
				if shape == "medium" {
					sku = "Standard_D4as_v5"
					meter = "D4as v5 Spot"
				}
				rate, err := lookup(priceQuery{Vendor: "azure", Region: bp.Region, Service: "Virtual Machines", PurchaseOption: "Spot", Unit: "1 Hour", Attributes: []priceAttribute{{Key: "armSkuName", Value: sku}, {Key: "meterName", Value: meter}, {Key: "productName", Value: "Virtual Machines Dasv5 Series"}}})
				if err != nil {
					return nil, err
				}
				for _, c := range pool.CostComponents {
					if strings.HasPrefix(c.Name, "Instance usage") {
						c.Name = "Instance usage (Linux, Spot, " + sku + ")"
						c.Price = decimalString(rate)
						if err := setQuantity(c, old); err != nil {
							return nil, err
						}
					}
				}
			}
			if bp.Cloud == "gcp" {
				rate, err := lookup(priceQuery{Vendor: "gcp", Region: bp.Region, Service: "Compute Engine", Unit: "gibibyte month", Attributes: []priceAttribute{{Key: "description", Value: "Balanced PD Capacity"}}})
				if err != nil {
					return nil, err
				}
				for _, c := range pool.CostComponents {
					if strings.Contains(c.Name, "provisioned storage") {
						q, err := number(*c.MonthlyQuantity, "node disk size")
						if err != nil {
							return nil, err
						}
						c.Name = "Balanced provisioned storage (pd-balanced)"
						c.Price = decimalString(rate)
						if err := setQuantity(c, q); err != nil {
							return nil, err
						}
					}
				}
			}
			if bp.Cloud == "aws" {
				rate, err := lookup(priceQuery{Vendor: "aws", Region: bp.Region, Service: "AmazonEC2", Family: "Storage", Unit: "GB-Mo", PurchaseOption: "on_demand", Attributes: []priceAttribute{{Key: "volumeApiName", Value: "gp3"}}})
				if err != nil {
					return nil, err
				}
				for _, c := range pool.CostComponents {
					if strings.HasPrefix(c.Name, "Storage (") {
						q, err := number(*c.MonthlyQuantity, "node disk size")
						if err != nil {
							return nil, err
						}
						c.Name = "Storage (AL2023 root volume, gp3)"
						c.Price = decimalString(rate)
						if err := setQuantity(c, q); err != nil {
							return nil, err
						}
					}
				}
			}
			if err := scaleResource(pool, hours/old); err != nil {
				return nil, err
			}
		}
	}
	if pools != 1 || clusters != 1 {
		return nil, fmt.Errorf("expected one catalog cluster and one autoscaled pool; found %d clusters and %d pools", clusters, pools)
	}
	return []string{"Kubernetes control planes and minimum nodes bill at idle. Additional node-hours apply per autoscaled pool, bounded by total minimum/maximum nodes; AKS Spot mode retains its separate on-demand system node. Spot prices are snapshots, not a future rate guarantee."}, nil
}

func correctCloud(d *breakdown, bp *blueprint.Blueprint, env blueprint.Environment, usage map[string]float64, mult float64, lookup productLookup) ([]string, error) {
	var notes []string
	n, err := correctObservability(d, bp, usage, mult, lookup)
	if err != nil {
		return nil, err
	}
	notes = append(notes, n...)
	n, err = correctServerless(d, bp, env, usage, mult, lookup)
	if err != nil {
		return nil, err
	}
	notes = append(notes, n...)
	n, err = correctNodes(d, bp, env, usage, mult, lookup)
	if err != nil {
		return nil, err
	}
	notes = append(notes, n...)
	n, err = correctNetwork(d, bp, env, usage, mult, lookup)
	if err != nil {
		return nil, err
	}
	notes = append(notes, n...)
	if bp.Cloud == "gcp" {
		for _, p := range d.Projects {
			for _, r := range p.Breakdown.Resources {
				if r.ResourceType == "google_storage_bucket" {
					for _, sub := range r.Subresources {
						if sub.Name == "Network egress" {
							for _, c := range sub.CostComponents {
								if c.MonthlyQuantity == nil {
									if err := setQuantity(c, 0); err != nil {
										return nil, err
									}
								}
							}
						}
					}
				}
			}
		}
		n, err = correctGCP(d, bp, env, usage, mult, gcpRateLookup(lookup))
		if err != nil {
			return nil, err
		}
		notes = append(notes, n...)
		count := 0
		for _, svc := range bp.Services {
			if svc.Kind == "cron" {
				count++
			}
		}
		count = min(count, d.Summary.UnsupportedResourceCounts["google_cloud_scheduler_job"])
		if count > 0 {
			rate, err := lookup(priceQuery{Vendor: "gcp", Service: "Cloud Scheduler", Region: "global", HighestTier: true, Attributes: []priceAttribute{{Key: "description", Value: "Jobs"}}})
			if err != nil {
				return nil, err
			}
			if err = addResource(d, &pricedResource{Name: "neckbeard.scheduler", ResourceType: "neckbeard_scheduler", CostComponents: []*costComponent{component("Scheduler (31 job-days per month)", "job-days", rate, float64(count)*31)}}); err != nil {
				return nil, err
			}
			modeledInstances(d, "google_cloud_scheduler_job", count)
		}
		for _, typ := range []string{"google_billing_budget", "google_cloud_run_v2_job_iam_member"} {
			if count := d.Summary.UnsupportedResourceCounts[typ]; count > 0 {
				if d.Summary.NoPriceResourceCounts == nil {
					d.Summary.NoPriceResourceCounts = map[string]int{}
				}
				d.Summary.NoPriceResourceCounts[typ] += count
				delete(d.Summary.UnsupportedResourceCounts, typ)
			}
		}
	}
	n, err = correctAzureDatabase(d, bp, env, lookup)
	if err != nil {
		return nil, err
	}
	notes = append(notes, n...)

	notes = append(notes, "Assumed unused: S3 Select, direct object-bucket egress, blob index/list extras, ACR cloud builds, Basic/Archive/export log features. Object reads/writes and log queries use named usage inputs; unconfigured optional features have explicit zero quantities.")
	return notes, d.recalculate()
}

// Account only for catalog-owned instances; unrelated user resources must keep
// their missing-price status even when they share a resource type.
func modeledInstances(d *breakdown, kind string, count int) {
	for _, counts := range []map[string]int{d.Summary.UnsupportedResourceCounts, d.Summary.NoPriceResourceCounts} {
		if counts == nil {
			continue
		}
		used := min(count, counts[kind])
		count -= used
		counts[kind] -= used
		if counts[kind] == 0 {
			delete(counts, kind)
		}
	}
}
