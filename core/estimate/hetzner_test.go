package estimate

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/blueprint"
)

func TestHetznerPriceTableLoadsAndIsDated(t *testing.T) {
	tbl, err := loadHetznerPriceTable()
	if err != nil {
		t.Fatal(err)
	}
	if tbl.Currency != "EUR" || len(tbl.Sources) < 2 || tbl.Servers["cx23"].Monthly <= 0 || tbl.LoadBalancers["lb11"].Monthly <= 0 {
		t.Fatalf("price table must carry EUR list prices with sources, got %+v", tbl)
	}
}

func hetznerEnv(withPostgres, withIngress bool) blueprint.Environment {
	env := blueprint.Environment{Name: "dev", Modules: []blueprint.ModuleUsage{
		{Name: "runtime-vps", Inputs: []blueprint.Input{{Key: "server_shape", Value: "small"}, {Key: "arch", Value: "x86"}}},
		{Name: "storage"},
	}}
	if withPostgres {
		env.Modules = append(env.Modules, blueprint.ModuleUsage{Name: "postgres", Inputs: []blueprint.Input{{Key: "volume_gb", Value: 20}}})
	}
	if withIngress {
		env.Modules = append(env.Modules, blueprint.ModuleUsage{Name: "dns-ingress"})
	}
	return env
}

func TestHetznerBreakdownIsListPriceArithmetic(t *testing.T) {
	tbl, err := loadHetznerPriceTable()
	if err != nil {
		t.Fatal(err)
	}
	bp := &blueprint.Blueprint{Cloud: "hetzner", Region: "fsn1", Runtime: "vps-containers"}
	doc, notes, err := hetznerBreakdown(bp, hetznerEnv(true, true), map[string]float64{"egress_gb": 20}, 1, tbl)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := number(doc.TotalMonthlyCost, "total")
	want := tbl.Servers["cx23"].Monthly + tbl.PrimaryIPv4Monthly + 20*tbl.VolumeGBMonthly + tbl.LoadBalancers["lb11"].Monthly + tbl.ObjectStorage.BaseMonthly
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("solo dev box: got %.4f want %.4f", got, want)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "Infracost does not price Hetzner") {
		t.Error("the notes must say where the numbers come from")
	}
	// Scenario scaling only touches traffic, and 20 GB is far inside the EU allowance.
	high, _, err := hetznerBreakdown(bp, hetznerEnv(true, true), map[string]float64{"egress_gb": 20}, 2, tbl)
	if err != nil {
		t.Fatal(err)
	}
	gotHigh, _ := number(high.TotalMonthlyCost, "total")
	if math.Abs(gotHigh-got) > 1e-9 {
		t.Fatalf("high scenario must not change a fixed-price box under the traffic allowance: %.4f vs %.4f", gotHigh, got)
	}
	// Singapore: 0.5 TB included, €7.40/TB after.
	sg := &blueprint.Blueprint{Cloud: "hetzner", Region: "sin", Runtime: "vps-containers"}
	heavy, _, err := hetznerBreakdown(sg, hetznerEnv(false, false), map[string]float64{"egress_gb": 1500}, 1, tbl)
	if err != nil {
		t.Fatal(err)
	}
	gotHeavy, _ := number(heavy.TotalMonthlyCost, "total")
	wantHeavy := tbl.Servers["cx23"].Monthly + tbl.PrimaryIPv4Monthly + tbl.ObjectStorage.BaseMonthly + (1.5-0.5)*tbl.Traffic.OveragePerTBSin
	if math.Abs(gotHeavy-wantHeavy) > 1e-9 {
		t.Fatalf("singapore overage: got %.4f want %.4f", gotHeavy, wantHeavy)
	}
	// Every component is priced: nothing unknown, so Complete() holds downstream.
	out, _ := json.Marshal(doc)
	_, _, _, unpriced, err := parseBreakdown(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range unpriced {
		if hasMissingPrice(u) {
			t.Errorf("price-table lane must not leave %q unpriced", u)
		}
	}
}

func TestHetznerRefusesUnknownShapes(t *testing.T) {
	tbl, _ := loadHetznerPriceTable()
	env := hetznerEnv(false, false)
	env.Modules[0].Inputs = []blueprint.Input{{Key: "server_shape", Value: "xlarge"}}
	if _, _, err := hetznerBreakdown(&blueprint.Blueprint{Cloud: "hetzner", Region: "fsn1"}, env, nil, 1, tbl); err == nil {
		t.Fatal("an unknown server shape must fail, not price as zero")
	}
}
