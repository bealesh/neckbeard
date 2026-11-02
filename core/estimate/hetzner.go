package estimate

import (
	_ "embed"
	"fmt"
	"math"

	"gopkg.in/yaml.v3"

	"github.com/bealesh/neckbeard/core/blueprint"
)

// Infracost prices AWS, Azure and Google only. The Hetzner lane is priced from
// a dated snapshot of public list prices instead (pricetables/hetzner.yaml):
// every line item below is a fixed monthly list price or a quantity times a
// list rate, and the report's provenance says "price table, as of <date>",
// not "Cloud Pricing API". Nothing here is a bill.

//go:embed pricetables/hetzner.yaml
var hetznerPriceTableYAML []byte

type hetznerServer struct {
	VCPU     int     `yaml:"vcpu"`
	MemoryGB int     `yaml:"memory_gb"`
	DiskGB   int     `yaml:"disk_gb"`
	Monthly  float64 `yaml:"monthly"`
	Arch     string  `yaml:"arch"`
}

type hetznerPriceTable struct {
	AsOf               string                   `yaml:"as_of"`
	Currency           string                   `yaml:"currency"`
	Sources            []string                 `yaml:"sources"`
	Servers            map[string]hetznerServer `yaml:"servers"`
	PrimaryIPv4Monthly float64                  `yaml:"primary_ipv4_monthly"`
	VolumeGBMonthly    float64                  `yaml:"volume_gb_monthly"`
	LoadBalancers      map[string]struct {
		Monthly           float64 `yaml:"monthly"`
		IncludedTrafficTB float64 `yaml:"included_traffic_tb"`
	} `yaml:"load_balancers"`
	ObjectStorage struct {
		BaseMonthly           float64 `yaml:"base_monthly"`
		IncludedStorageTB     float64 `yaml:"included_storage_tb"`
		IncludedEgressTB      float64 `yaml:"included_egress_tb"`
		AdditionalStorageHour float64 `yaml:"additional_storage_tb_hour"`
		AdditionalEgressTB    float64 `yaml:"additional_egress_tb"`
	} `yaml:"object_storage"`
	Traffic struct {
		OveragePerTB       float64            `yaml:"overage_per_tb"`
		IncludedTBByRegion map[string]float64 `yaml:"included_tb_by_region"`
		OveragePerTBSin    float64            `yaml:"overage_per_tb_sin"`
	} `yaml:"traffic"`
}

func loadHetznerPriceTable() (*hetznerPriceTable, error) {
	var t hetznerPriceTable
	if err := yaml.Unmarshal(hetznerPriceTableYAML, &t); err != nil {
		return nil, fmt.Errorf("hetzner price table: %w", err)
	}
	if t.AsOf == "" || t.Currency == "" || len(t.Servers) == 0 {
		return nil, fmt.Errorf("hetzner price table is incomplete")
	}
	return &t, nil
}

// hetznerServerType mirrors the runtime-vps module's shape/arch → type table.
func hetznerServerType(shape, arch string) string {
	x86 := map[string]string{"small": "cx23", "medium": "cx33", "large": "cx43"}
	arm := map[string]string{"small": "cax11", "medium": "cax21", "large": "cax31"}
	if arch == "arm" {
		return arm[shape]
	}
	return x86[shape]
}

// hetznerBreakdown builds a fully priced breakdown document for one
// environment from the blueprint's module inputs and the price table. It is
// the Hetzner stand-in for `infracost breakdown`; the rest of the estimate
// pipeline (parseBreakdown, Complete, the report) is unchanged downstream.
func hetznerBreakdown(bp *blueprint.Blueprint, env blueprint.Environment, usage map[string]float64, mult float64, t *hetznerPriceTable) (*breakdown, []string, error) {
	var d breakdown
	d.Projects = make([]struct {
		Breakdown struct {
			Resources []*pricedResource `json:"resources"`
		} `json:"breakdown"`
	}, 1)
	present := map[string]bool{}
	for _, m := range env.Modules {
		present[m.Name] = true
	}
	var notes []string

	// Server: one per environment, always on, plus its primary IPv4.
	rt := moduleInputs(env, "runtime-vps")
	shape := fmt.Sprint(valueOr(rt["server_shape"], "small"))
	arch := fmt.Sprint(valueOr(rt["arch"], "x86"))
	serverType := hetznerServerType(shape, arch)
	server, ok := t.Servers[serverType]
	if !ok {
		return nil, nil, fmt.Errorf("hetzner price table has no entry for server type %q (shape %s, %s)", serverType, shape, arch)
	}
	if err := addResource(&d, &pricedResource{Name: "module.runtime_vps.hcloud_server.app", ResourceType: "hcloud_server", CostComponents: []*costComponent{
		component(fmt.Sprintf("Server %s (%d vCPU, %d GB, %d GB disk, always on)", serverType, server.VCPU, server.MemoryGB, server.DiskGB), "months", server.Monthly, 1),
		component("Primary IPv4", "months", t.PrimaryIPv4Monthly, 1),
	}}); err != nil {
		return nil, nil, err
	}

	// Postgres data volume.
	if present["postgres"] {
		pg := moduleInputs(env, "postgres")
		gb, err := inputNumber(pg, "volume_gb")
		if err != nil {
			return nil, nil, err
		}
		if err := addResource(&d, &pricedResource{Name: "module.postgres.hcloud_volume.data", ResourceType: "hcloud_volume", CostComponents: []*costComponent{
			component("Block storage for PGDATA", "GB months", t.VolumeGBMonthly, gb),
		}}); err != nil {
			return nil, nil, err
		}
	}

	// Load balancer, when there is an HTTP service to front.
	if present["dns-ingress"] {
		lbType := fmt.Sprint(valueOr(moduleInputs(env, "dns-ingress")["load_balancer_type"], "lb11"))
		lb, ok := t.LoadBalancers[lbType]
		if !ok {
			return nil, nil, fmt.Errorf("hetzner price table has no entry for load balancer type %q", lbType)
		}
		if err := addResource(&d, &pricedResource{Name: "module.dns_ingress.hcloud_load_balancer.this", ResourceType: "hcloud_load_balancer", CostComponents: []*costComponent{
			component(fmt.Sprintf("Load balancer %s (%g TB traffic included)", lbType, lb.IncludedTrafficTB), "months", lb.Monthly, 1),
		}}); err != nil {
			return nil, nil, err
		}
	}

	// Object Storage: base price includes 1 TB stored and 1 TB egress. Billed
	// once per project; this lane uses one project per environment, so it is
	// charged here per environment.
	if present["storage"] {
		if err := addResource(&d, &pricedResource{Name: "module.storage.minio_s3_bucket.data", ResourceType: "minio_s3_bucket", CostComponents: []*costComponent{
			component(fmt.Sprintf("Object Storage base (%g TB stored, %g TB egress included)", t.ObjectStorage.IncludedStorageTB, t.ObjectStorage.IncludedEgressTB), "months", t.ObjectStorage.BaseMonthly, 1),
		}}); err != nil {
			return nil, nil, err
		}
		notes = append(notes, "Object Storage is billed per project; with one project per environment the base price appears once per environment. Buckets share the included terabyte.")
	}

	// Internet traffic beyond the server's included allowance, scaled by scenario.
	egressGB := usage["egress_gb"] * mult
	includedTB := t.Traffic.IncludedTBByRegion[bp.Region]
	rate := t.Traffic.OveragePerTB
	if bp.Region == "sin" {
		rate = t.Traffic.OveragePerTBSin
	}
	overTB := math.Max(0, egressGB/1000-includedTB)
	if err := addResource(&d, &pricedResource{Name: "neckbeard.network_transfer", ResourceType: "neckbeard_network_transfer", CostComponents: []*costComponent{
		{Name: fmt.Sprintf("Internet transfer above the %g TB included with the server", includedTB), Unit: "TB", Price: decimalString(rate), MonthlyQuantity: decimalString(overTB), MonthlyCost: decimalString(rate * overTB), UsageBased: true},
	}}); err != nil {
		return nil, nil, err
	}

	notes = append(notes, fmt.Sprintf("Hetzner list prices excluding VAT as of %s (%s), for new orders after the 2026-06-15 repricing; Infracost does not price Hetzner. Verify against your account's pricing page.", t.AsOf, t.Currency))
	return &d, notes, d.recalculate()
}

func valueOr(v any, fallback any) any {
	if v == nil || fmt.Sprint(v) == "" {
		return fallback
	}
	return v
}
