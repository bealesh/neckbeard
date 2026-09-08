package estimate

import (
	"fmt"
	"strings"
	"time"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/config"
)

// Report renders the five-section cost report (DESIGN §13.2). It is a
// point-in-time artifact, not a generated-set member: prices move, so the footer
// carries provenance and freshness instead of pretending stability.
func Report(res *Result, cfg *config.Config, bp *blueprint.Blueprint, now time.Time) []byte {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	money := func(v float64) string { return fmt.Sprintf("%.2f %s", v, res.Currency) }

	w("# Cost estimate — %s", bp.App)
	w("")
	w("Lane %s / %s / tier %s, region %s. Estimated from rendered HCL (no cloud", bp.Cloud, bp.Runtime, bp.Tier, bp.Region)
	w("credentials); an authenticated plan (V1) improves resolution but was not used.")
	w("")

	w("## 1. Baseline provisioned (bills while idle)")
	w("")
	w("| environment | monthly |")
	w("|---|---|")
	var baselineTotal float64
	for _, e := range res.Envs {
		w("| %s | %s |", e.Env, money(e.Costs["baseline"]))
		baselineTotal += e.Costs["baseline"]
	}
	w("| **total** | **%s** |", money(baselineTotal))
	w("")

	w("## 2. Usage assumptions (explicit numbers — a tier name is not a usage estimate)")
	w("")
	w("Every number is editable in `neckbeard.yaml` under `finops.usage`.")
	w("")
	w("| assumption | monthly value | source |")
	w("|---|---|---|")
	for _, k := range sortedKeys(res.Usage) {
		w("| %s | %g | %s |", k, res.Usage[k], res.UsageSource[k])
	}
	w("")

	w("## 3. Scenario ranges (monthly)")
	w("")
	w("| environment | low (0.5×) | expected (1×) | high (2×) |")
	w("|---|---|---|---|")
	var lo, mid, hi float64
	for _, e := range res.Envs {
		w("| %s | %s | %s | %s |", e.Env, money(e.Costs["low"]), money(e.Costs["expected"]), money(e.Costs["high"]))
		lo += e.Costs["low"]
		mid += e.Costs["expected"]
		hi += e.Costs["high"]
	}
	w("| **total** | **%s** | **%s** | **%s** | ", money(lo), money(mid), money(hi))
	w("")
	w("Yearly at expected usage: **%s**.", money(mid*12))
	w("")
	if alert := cfg.FinOps.MonthlyBudgetAlert; alert > 0 {
		rel := "within"
		if mid > alert {
			rel = "OVER"
		}
		w("Budget alert threshold: %s — the expected total is **%s** it. Budget alerts", money(alert), rel)
		w("notify; they do not cap spending.")
		w("")
	}

	w("### Largest expected line items")
	w("")
	for _, e := range res.Envs {
		w("**%s**", e.Env)
		w("")
		n := 0
		for _, r := range e.Resources {
			if r.Monthly <= 0 || n >= 5 {
				continue
			}
			w("- %s — %s", r.Address, money(r.Monthly))
			n++
		}
		w("")
	}

	w("## 4. Shared platform costs")
	w("")
	if cfg.EntryPath == "found" {
		w("Landing-zone costs (audit log storage, org guardrail tooling) are estimated")
		w("with the platform repo, not here.")
	} else {
		w("None in this estimate: adopt path, no neckbeard-managed landing zone. Central")
		w("costs your organization already carries (audit logging, org tooling) are not")
		w("represented here.")
	}
	w("")

	w("## 5. Unpriced and unmodeled items (named, never silently dropped)")
	w("")
	seen := map[string]bool{}
	for _, e := range res.Envs {
		for _, u := range e.Unpriced {
			if !seen[u] {
				w("- %s", u)
				seen[u] = true
			}
		}
	}
	for _, k := range res.UnmappedUsage {
		w("- usage assumption `%s` has no priceable IaC resource in this lane (e.g. request-path egress); treat the high scenario as its buffer", k)
	}
	if len(seen) == 0 && len(res.UnmappedUsage) == 0 {
		w("- none detected")
	}
	w("")

	w("---")
	w("")
	w("Provenance: infracost %s (Cloud Pricing API) · catalog %s · planner %s ·", res.InfracostVersion, bp.Pins.Catalog, bp.Pins.Planner)
	w("blueprint `%s` · generated %s.", bp.Hash, now.UTC().Format("2006-01-02 15:04 UTC"))
	w("Estimates are estimates: provisioned prices are firm at snapshot time; every")
	w("usage-driven line is only as good as section 2's assumptions.")
	return []byte(b.String())
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}
