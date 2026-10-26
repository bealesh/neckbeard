package estimate

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/catalog"
	"github.com/bealesh/neckbeard/core/config"
	"github.com/bealesh/neckbeard/core/doctor"
	"github.com/bealesh/neckbeard/core/planner"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/core/profile"
	"github.com/bealesh/neckbeard/core/render"
)

type priceFact struct {
	Query priceQuery
	Rate  float64
}
type matrixFixture struct {
	Currency    string
	Blueprint   blueprint.Blueprint
	Environment blueprint.Environment
	Usage       map[string]float64
	Native      json.RawMessage
	Prices      []priceFact
	Totals      map[string]float64
}

func TestRecordedCostMatrix(t *testing.T) {
	files, err := filepath.Glob("testdata/matrix/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 12 {
		t.Fatalf("need USD and EUR coverage for all six lanes, got %d fixtures", len(files))
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var fixture matrixFixture
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}
			facts := map[string]float64{}
			for _, fact := range fixture.Prices {
				key, _ := json.Marshal(fact.Query)
				facts[string(key)] = fact.Rate
			}
			lookup := func(q priceQuery) (float64, error) {
				key, _ := json.Marshal(q)
				rate, ok := facts[string(key)]
				if !ok {
					return 0, fmt.Errorf("unrecorded price query: %s", key)
				}
				return rate, nil
			}
			var doc breakdown
			if err = json.Unmarshal(fixture.Native, &doc); err != nil {
				t.Fatal(err)
			}
			if _, err = correctCloud(&doc, &fixture.Blueprint, fixture.Environment, fixture.Usage, 1, lookup); err != nil {
				t.Fatal(err)
			}
			out, _ := json.Marshal(doc)
			total, _, _, unknown, err := parseBreakdown(out)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range unknown {
				if hasMissingPrice(item) {
					t.Errorf("unpriced component: %s", item)
				}
			}
			if math.Abs(total-fixture.Totals["expected"]) > 1e-7 {
				t.Fatalf("got %.9f, recorded %.9f", total, fixture.Totals["expected"])
			}
		})
	}
}

func hasMissingPrice(item string) bool {
	return strings.Contains(item, "missing price or usage quantity") || strings.Contains(item, "unsupported by infracost")
}

// Opt-in price-only refresh; no provider credentials or deployments. Raw fixture
// serialization strips tags, account data, source paths, and Terraform metadata.
func TestLiveCostMatrix(t *testing.T) {
	root := os.Getenv("NECKBEARD_COST_EVIDENCE_ROOT")
	if root == "" {
		t.Skip("set NECKBEARD_COST_EVIDENCE_ROOT to the six locally scaffolded reference lanes")
	}
	bin, err := doctor.Infracost("")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := presets.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, cloud := range []string{"aws", "azure", "gcp"} {
		for _, runtime := range []string{"serverless-containers", "kubernetes"} {
			for _, currency := range []string{"USD", "EUR"} {
				name := cloud + "-" + runtime + "-" + currency
				t.Run(name, func(t *testing.T) {
					dir := filepath.Join(root, cloud+"-"+runtime)
					if err := prepareReferenceLane(dir, cloud, runtime, pre); err != nil {
						t.Fatal(err)
					}
					bp, err := blueprint.Load(filepath.Join(dir, "blueprint.yaml"))
					if err != nil {
						t.Fatal(err)
					}
					cfg, _, err := config.Load(filepath.Join(dir, "neckbeard.yaml"))
					if err != nil {
						t.Fatal(err)
					}
					usage, _ := resolveUsage(pre.Tiers[bp.Tier], cfg)
					env := bp.Environments[0]
					products, err := pricingLookup(bin, currency)
					if err != nil {
						t.Fatal(err)
					}
					facts := map[string]priceFact{}
					lookup := func(q priceQuery) (float64, error) {
						price, err := products(q)
						if err == nil {
							key, _ := json.Marshal(q)
							facts[string(key)] = priceFact{q, price}
						}
						return price, err
					}
					fixture := matrixFixture{Currency: currency, Blueprint: blueprint.Blueprint{Cloud: bp.Cloud, Region: bp.Region, Runtime: bp.Runtime, Services: bp.Services}, Environment: env, Usage: usage, Totals: map[string]float64{}}
					scratch := t.TempDir()
					for _, sc := range scenarios {
						uf, err := BuildUsageFile(usage, sc.Multiplier)
						if err != nil {
							t.Fatal(err)
						}
						usagePath := filepath.Join(scratch, "usage.yml")
						if err = os.WriteFile(usagePath, uf, 0600); err != nil {
							t.Fatal(err)
						}
						out, err := runInfracost(bin, currency, "breakdown", "--path", filepath.Join(dir, "infra/envs", env.Name), "--format", "json", "--usage-file", usagePath)
						if err != nil {
							t.Fatal(err)
						}
						var doc breakdown
						if err = json.Unmarshal(out, &doc); err != nil {
							t.Fatal(err)
						}
						if sc.Name == "expected" {
							fixture.Native, _ = json.Marshal(doc)
						}
						if _, err = correctCloud(&doc, bp, env, usage, sc.Multiplier, lookup); err != nil {
							t.Fatal(err)
						}
						out, _ = json.Marshal(doc)
						total, _, _, unknown, err := parseBreakdown(out)
						if err != nil {
							t.Fatal(err)
						}
						for _, item := range unknown {
							if hasMissingPrice(item) {
								t.Errorf("%s: %s", sc.Name, item)
							}
						}
						fixture.Totals[sc.Name] = total
					}
					keys := []string{}
					for key := range facts {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					for _, key := range keys {
						fixture.Prices = append(fixture.Prices, facts[key])
					}
					// Keep only the non-sensitive module settings the cost model reads.
					allowed := map[string]bool{"cpu": true, "memory_gb": true, "min_instances": true, "max_instances": true, "min_nodes": true, "max_nodes": true, "spot": true, "node_shape": true, "zones": true, "read_replicas": true, "multi_zone": true}
					for i, m := range fixture.Environment.Modules {
						inputs := []blueprint.Input{}
						for _, in := range m.Inputs {
							if allowed[in.Key] {
								inputs = append(inputs, blueprint.Input{Key: in.Key, Value: in.Value})
							}
						}
						fixture.Environment.Modules[i] = blueprint.ModuleUsage{Name: m.Name, Inputs: inputs}
					}
					fixture.Environment.Container = ""
					t.Logf("%s baseline %.6f low %.6f expected %.6f high %.6f", currency, fixture.Totals["baseline"], fixture.Totals["low"], fixture.Totals["expected"], fixture.Totals["high"])
					if os.Getenv("NECKBEARD_WRITE_COST_FIXTURES") == "1" && !t.Failed() {
						if err = os.MkdirAll("testdata/matrix", 0755); err != nil {
							t.Fatal(err)
						}
						data, _ := json.MarshalIndent(fixture, "", "  ")
						if err = os.WriteFile(filepath.Join("testdata/matrix", name+".json"), append(data, '\n'), 0644); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func prepareReferenceLane(dir, cloud, runtime string, pre *presets.Set) error {
	if _, err := os.Stat(filepath.Join(dir, "blueprint.yaml")); err == nil {
		return nil
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("reference lane %s is nonempty and has no blueprint", dir)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := os.ReadFile("../../bellwether/neckbeard.yaml")
	if err != nil {
		return err
	}
	cfgText := strings.ReplaceAll(string(data), "cloud: aws", "cloud: "+cloud)
	cfgText = strings.ReplaceAll(cfgText, "runtime: serverless-containers", "runtime: "+runtime)
	region := map[string]string{"aws": "us-east-1", "azure": "eastus", "gcp": "us-central1"}[cloud]
	cfgText = strings.ReplaceAll(cfgText, "region: us-east-1", "region: "+region)
	if cloud == "gcp" {
		cfgText = strings.ReplaceAll(cfgText, "  currency: USD", "  currency: USD\n  billing_account: \"012345-6789AB-CDEF01\"")
		cfgText += "\ncontainers: {dev: cost-dev, stg: cost-stg, prd: cost-prd}\n"
	}
	if cloud == "azure" {
		cfgText += "\ncontainers: {dev: 00000000-0000-0000-0000-000000000001, stg: 00000000-0000-0000-0000-000000000002, prd: 00000000-0000-0000-0000-000000000003}\n"
	}
	cfgPath := filepath.Join(dir, "neckbeard.yaml")
	if err = os.WriteFile(cfgPath, []byte(cfgText), 0600); err != nil {
		return err
	}
	cfg, digest, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	prof, pdigest, err := profile.Load("../planner/testdata/basic/app-profile.yaml")
	if err != nil {
		return err
	}
	cat, err := catalog.Load("")
	if err != nil {
		return err
	}
	bp, err := planner.Plan(planner.Inputs{Config: cfg, ConfigDigest: digest, Profile: prof, ProfileDigest: pdigest, Catalog: cat, Presets: pre, PlannerVersion: "cost-reference"})
	if err != nil {
		return err
	}
	bytes, err := bp.Finalize()
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "blueprint.yaml"), bytes, 0600); err != nil {
		return err
	}
	files, err := render.WriteSet(bp, render.Options{})
	if err != nil {
		return err
	}
	return writeScratch(dir, files)
}
