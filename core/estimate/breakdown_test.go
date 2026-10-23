package estimate

import "testing"

func TestBreakdownRejectsInvalidMoney(t *testing.T) {
	for _, data := range []string{`{}`, `{"totalMonthlyCost":"oops"}`, `{"totalMonthlyCost":"NaN"}`, `{"totalMonthlyCost":"-1"}`, `{"totalMonthlyCost":"+Inf"}`} {
		if _, _, _, _, err := parseBreakdown([]byte(data)); err == nil {
			t.Errorf("accepted invalid pricing output %s", data)
		}
	}
}

func TestBreakdownPreservesUnpricedComponents(t *testing.T) {
	data := []byte(`{"totalMonthlyCost":"1","projects":[{"breakdown":{"resources":[{"name":"app","resourceType":"google_cloud_run_v2_service","monthlyCost":"1","costComponents":[{"name":"CPU","monthlyQuantity":null,"monthlyCost":null}]}]}}]}`)
	_, _, _, unknown, err := parseBreakdown(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown) == 0 {
		t.Fatal("missing compute quantity was silently treated as zero")
	}
}
