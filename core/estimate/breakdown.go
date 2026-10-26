package estimate

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

type breakdown struct {
	TotalMonthlyCost string `json:"totalMonthlyCost"`
	Projects         []struct {
		Breakdown struct {
			Resources []*pricedResource `json:"resources"`
		} `json:"breakdown"`
	} `json:"projects"`
	Summary struct {
		UnsupportedResourceCounts map[string]int `json:"unsupportedResourceCounts"`
		NoPriceResourceCounts     map[string]int `json:"noPriceResourceCounts"`
	} `json:"summary"`
}

type pricedResource struct {
	Name           string            `json:"name"`
	ResourceType   string            `json:"resourceType"`
	MonthlyCost    *string           `json:"monthlyCost"`
	CostComponents []*costComponent  `json:"costComponents"`
	Subresources   []*pricedResource `json:"subresources"`
}

type costComponent struct {
	Name            string  `json:"name"`
	Unit            string  `json:"unit"`
	Price           *string `json:"price"`
	MonthlyQuantity *string `json:"monthlyQuantity"`
	MonthlyCost     *string `json:"monthlyCost"`
	UsageBased      bool    `json:"usageBased"`
	PriceNotFound   bool    `json:"priceNotFound"`
}

func number(s, what string) (float64, error) {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, fmt.Errorf("invalid %s in pricing output", what)
	}
	return n, nil
}

func decimalString(n float64) *string { s := strconv.FormatFloat(n, 'f', -1, 64); return &s }

func (r *pricedResource) total() (float64, error) {
	if r == nil {
		return 0, fmt.Errorf("null resource in pricing output")
	}
	var total float64
	for _, c := range r.CostComponents {
		if c == nil {
			return 0, fmt.Errorf("null cost component in pricing output")
		}
		if c.MonthlyCost != nil {
			n, err := number(*c.MonthlyCost, "component cost")
			if err != nil {
				return 0, err
			}
			total += n
		}
	}
	for _, child := range r.Subresources {
		n, err := child.total()
		if err != nil {
			return 0, err
		}
		total += n
	}
	r.MonthlyCost = decimalString(total)
	return total, nil
}

func (d *breakdown) recalculate() error {
	var total float64
	for _, p := range d.Projects {
		for _, r := range p.Breakdown.Resources {
			n, err := r.total()
			if err != nil {
				return err
			}
			total += n
		}
	}
	d.TotalMonthlyCost = strconv.FormatFloat(total, 'f', -1, 64)
	return nil
}

func parseBreakdown(out []byte) (total float64, resources []ResourceCost, resourceTypes map[string]bool, unpriced []string, err error) {
	var doc breakdown
	if err = json.Unmarshal(out, &doc); err != nil {
		return 0, nil, nil, nil, fmt.Errorf("parsing infracost JSON: %w", err)
	}
	total, err = number(doc.TotalMonthlyCost, "total monthly cost")
	if err != nil {
		return 0, nil, nil, nil, err
	}
	resourceTypes = map[string]bool{}
	var inspect func(*pricedResource) error
	inspect = func(r *pricedResource) error {
		if r == nil {
			return fmt.Errorf("null resource in pricing output")
		}
		for _, c := range r.CostComponents {
			if c == nil {
				return fmt.Errorf("null cost component in pricing output")
			}
			for _, value := range []*string{c.Price, c.MonthlyQuantity, c.MonthlyCost} {
				if value != nil {
					if _, e := number(*value, "component amount"); e != nil {
						return e
					}
				}
			}
			if c.PriceNotFound || c.Price == nil || c.MonthlyQuantity == nil || c.MonthlyCost == nil {
				unpriced = append(unpriced, r.Name+": "+c.Name+" (missing price or usage quantity)")
				continue
			}
			for _, s := range []string{*c.Price, *c.MonthlyQuantity, *c.MonthlyCost} {
				if _, e := number(s, "component amount"); e != nil {
					return e
				}
			}
		}
		for _, child := range r.Subresources {
			if e := inspect(child); e != nil {
				return e
			}
		}
		return nil
	}
	for _, p := range doc.Projects {
		for _, r := range p.Breakdown.Resources {
			if r == nil {
				return 0, nil, nil, nil, fmt.Errorf("null resource in pricing output")
			}
			cost := 0.0
			if r.MonthlyCost != nil {
				cost, err = number(*r.MonthlyCost, "resource cost")
				if err != nil {
					return 0, nil, nil, nil, err
				}
			}
			resources = append(resources, ResourceCost{Address: r.Name, Monthly: cost})
			resourceTypes[r.ResourceType] = true
			if err = inspect(r); err != nil {
				return 0, nil, nil, nil, err
			}
		}
	}
	if len(resources) == 0 {
		return 0, nil, nil, nil, fmt.Errorf("pricing output contains no resources")
	}
	sortResources(resources)
	for name, n := range doc.Summary.UnsupportedResourceCounts {
		unpriced = append(unpriced, fmt.Sprintf("%s ×%d (unsupported by infracost)", name, n))
	}
	// These resources have no separate charge in the upstream model; do not call
	// the whole service free merely because its control-plane object is unpriced.
	for name, n := range doc.Summary.NoPriceResourceCounts {
		if billableResource(name) {
			unpriced = append(unpriced, fmt.Sprintf("%s ×%d (missing price or usage quantity)", name, n))
		} else {
			unpriced = append(unpriced, fmt.Sprintf("%s ×%d (no separate modeled charge)", name, n))
		}
	}
	sortStrings(unpriced)
	return total, resources, resourceTypes, unpriced, nil
}

func billableResource(kind string) bool {
	switch kind {
	case "azurerm_postgresql_flexible_server", "azurerm_container_app", "azurerm_container_app_job", "azurerm_container_app_environment", "azurerm_kubernetes_cluster", "azurerm_kubernetes_cluster_node_pool", "aws_ecs_service", "aws_eks_node_group", "google_cloud_run_v2_service", "google_cloud_run_v2_job", "google_cloud_run_v2_worker_pool", "google_container_cluster", "google_container_node_pool":
		return true
	}
	return false
}
