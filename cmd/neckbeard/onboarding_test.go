package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/profile"
	"gopkg.in/yaml.v3"
)

// Run the built distribution from unrelated app directories, never using a
// source-checkout catalog flag. Pricing is stubbed; this is not a price/live test.
func TestInstalledOnboarding(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		unit := "1 Hour"
		if bytes.Contains(body, []byte("Bandwidth")) || bytes.Contains(body, []byte("Standard Data Processed")) {
			unit = "1 GB"
		}
		_, _ = fmt.Fprintf(w, `{"data":{"products":[{"prices":[{"USD":"0.1","unit":%q}]}]}}`, unit)
	}))
	defer pricing.Close()
	t.Setenv("INFRACOST_API_KEY", "onboarding-fixture")
	t.Setenv("INFRACOST_PRICING_API_ENDPOINT", pricing.URL)

	bin := filepath.Join(t.TempDir(), "neckbeard")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, shape := range []string{"no-dockerfile", "multi-image"} {
		t.Run(shape, func(t *testing.T) {
			app := t.TempDir()
			if shape == "multi-image" {
				// No repository-root Dockerfile: with no convention to prefer, two
				// candidates stay two images and the workload gate refuses. (A root
				// Dockerfile plus extras is the root-preference path, covered in
				// core/analyze tests: the extras become a plan-gating assumption.)
				for _, dir := range []string{"web", "worker"} {
					if err := os.Mkdir(filepath.Join(app, dir), 0755); err != nil {
						t.Fatal(err)
					}
					writeTest(t, filepath.Join(app, dir, "Dockerfile"), []byte("FROM node:22-alpine\nEXPOSE 3000\n"))
				}
			}
			cmd := exec.Command(bin, "analyze")
			cmd.Dir = app
			out, err := cmd.CombinedOutput()
			if err == nil || !bytes.Contains(out, []byte("before cloud configuration")) || bytes.Contains(out, []byte("OPEN QUESTIONS")) {
				t.Fatalf("unsupported workload reached cloud questions: %v\n%s", err, out)
			}
			if _, err := os.Stat(filepath.Join(app, "app-profile.yaml")); err != nil {
				t.Fatal("draft evidence was not saved:", err)
			}
		})
	}
	for _, fixture := range []string{"node", "python", "unsupported"} {
		t.Run(fixture, func(t *testing.T) {
			app := t.TempDir()
			entries, err := os.ReadDir(filepath.Join("testdata", "onboarding", fixture))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				data, err := os.ReadFile(filepath.Join("testdata", "onboarding", fixture, e.Name()))
				if err != nil {
					t.Fatal(err)
				}
				writeTest(t, filepath.Join(app, e.Name()), data)
			}
			run := func(ok bool, args ...string) string {
				t.Helper()
				cmd := exec.Command(bin, args...)
				cmd.Dir = app
				out, err := cmd.CombinedOutput()
				if (err == nil) != ok {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return string(out)
			}
			run(true, "doctor", "-for", "plan")
			run(true, "skill", "install")
			for _, ref := range []string{"onboarding.md", "app-profile.schema.json", "neckbeard.schema.json", "presets.yaml", "catalog.yaml"} {
				if _, err := os.Stat(filepath.Join(app, ".agents/skills/neckbeard/references", ref)); err != nil {
					t.Fatal(err)
				}
			}
			run(true, "analyze")
			p, _, err := profile.Load(filepath.Join(app, "app-profile.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			config := func(cloud, vcs, runtime string) {
				region := map[string]string{"aws": "us-east-1", "gcp": "us-central1", "azure": "centralus"}[cloud]
				data := fmt.Sprintf("version: 1\napp: example\norg: acme\ncloud: %s\nregion: %s\nvcs: %s\nrepo: acme/example\nruntime: %s\ntier: smallteam\nentry_path: adopt\n", cloud, region, vcs, runtime)
				if cloud != "aws" {
					data += "containers: {dev: example-dev, stg: example-stg, prd: example-prd}\n"
				}
				writeTest(t, filepath.Join(app, "neckbeard.yaml"), []byte(data))
			}
			config("aws", "github", "serverless-containers")
			if out := run(false, "plan"); !strings.Contains(out, "assumption") {
				t.Fatalf("draft unexpectedly accepted: %s", out)
			}
			for _, a := range p.Assumptions {
				p.Confirmed = append(p.Confirmed, profile.Confirmed{ID: a.ID, Question: a.Statement, Answer: "Fixture reviewed: one HTTP process, existing dependencies, smallteam configuration."})
			}
			for i := range p.Needs {
				if p.Needs[i].Capability == "postgres" {
					p.Needs[i].Mode = "reference"
					p.Needs[i].SecretName = "DATABASE_URL"
				}
			}
			saveProfile := func() {
				data, err := yaml.Marshal(p)
				if err != nil {
					t.Fatal(err)
				}
				writeTest(t, filepath.Join(app, "app-profile.yaml"), data)
			}
			saveProfile()
			if fixture == "unsupported" {
				if out := run(false, "plan"); !strings.Contains(out, "redis: choose mode") {
					t.Fatalf("Redis mode was not gated: %s", out)
				}
				for i := range p.Needs {
					if p.Needs[i].Capability == "redis" {
						p.Needs[i].Mode = "provision"
					}
				}
				saveProfile()
				if out := run(false, "plan"); !strings.Contains(out, "force-fit") {
					t.Fatalf("cloud Redis provisioning was not refused: %s", out)
				}
				for i := range p.Needs {
					if p.Needs[i].Capability == "redis" {
						p.Needs[i].Mode = "reference"
						p.Needs[i].SecretName = "REDIS_URL"
					}
				}
				saveProfile()
			}

			for _, cloud := range []string{"aws", "gcp", "azure"} {
				for _, vcs := range []string{"github", "gitlab"} {
					for _, runtime := range []string{"serverless-containers", "kubernetes"} {
						config(cloud, vcs, runtime)
						run(true, "plan")
						first, err := os.ReadFile(filepath.Join(app, "blueprint.yaml"))
						if err != nil {
							t.Fatal(err)
						}
						run(true, "plan")
						second, _ := os.ReadFile(filepath.Join(app, "blueprint.yaml"))
						if !bytes.Equal(first, second) {
							t.Fatal("plan is not deterministic")
						}
						bp, err := blueprint.Load(filepath.Join(app, "blueprint.yaml"))
						if err != nil {
							t.Fatal(err)
						}
						if len(bp.Services[0].Args) != 0 {
							t.Fatalf("image defaults overridden: %v", bp.Services[0].Args)
						}
						if fixture == "python" {
							for _, e := range bp.Environments {
								for _, m := range e.Modules {
									if m.Name == "postgres" {
										t.Fatal("existing database reprovisioned")
									}
								}
							}
						}
						if fixture == "unsupported" && (len(bp.References) != 1 || bp.References[0].SecretName != "REDIS_URL") {
							t.Fatal("external disposition was lost")
						}
						run(true, "scaffold")
						module := filepath.Join(app, "infra/envs/dev/main.tf")
						data, _ := os.ReadFile(module)
						if !bytes.Contains(data, []byte("../../../.neckbeard/catalog/")) {
							t.Fatal("scaffold depends on checkout or remote tag")
						}
						if runtime == "kubernetes" {
							data, _ = os.ReadFile(filepath.Join(app, "clusters/base/apps/deployment-web.yaml"))
							if bytes.Contains(data, []byte("args:")) {
								t.Fatal("Kubernetes overrides image CMD")
							}
						}
					}
				}
			}
			fake := filepath.Join(t.TempDir(), "infracost")
			writeTest(t, fake, []byte(fmt.Sprintf(`#!/bin/sh
if [ "$1" = --version ]; then echo 'Infracost v0.10.45'; exit 0; fi
# Verify the estimator receives a self-contained scratch tree, not just env roots.
[ "$1" = breakdown ] && [ "$2" = --path ] || exit 2
project_path="$3"
set -- "$3"/../../../.neckbeard/catalog/*/catalog/azure/runtime-k8s/main.tf
[ -f "$1" ] || { echo 'bundled catalog missing from estimate scratch' >&2; exit 3; }
case "$project_path" in
 */dev) echo '%s' ;;
 *) echo '%s' ;;
esac
`, onboardingPriceFixture(true), onboardingPriceFixture(false))))
			if err := os.Chmod(fake, 0755); err != nil {
				t.Fatal(err)
			}
			run(true, "estimate", "-infracost-bin", fake)
			if _, err := os.Stat(filepath.Join(app, "costs/estimate.md")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func writeTest(t *testing.T, p string, data []byte) {
	t.Helper()
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// The final onboarding loop selects AKS. Keep the pricing stub structurally
// faithful; the estimator now rejects a missing cluster or node pool.
func onboardingPriceFixture(spot bool) string {
	comp := func(name, q, p, cost string) map[string]any {
		return map[string]any{"name": name, "monthlyQuantity": q, "price": p, "monthlyCost": cost}
	}
	pool := func(name, kind string) map[string]any {
		return map[string]any{"name": name, "resourceType": kind, "costComponents": []any{comp("Instance usage (Linux)", "730", "0.086", "62.78")}, "subresources": []any{map[string]any{"name": "os_disk", "costComponents": []any{comp("Storage (P10)", "1", "19.71", "19.71")}}}}
	}
	cluster := map[string]any{"name": "module.runtime_k8s.azurerm_kubernetes_cluster.this", "resourceType": "azurerm_kubernetes_cluster", "costComponents": []any{comp("Control plane", "730", "0.1", "73")}, "subresources": []any{pool("default_node_pool", "")}}
	resources := []any{cluster}
	total := "155.49"
	if spot {
		resources = append(resources, pool("module.runtime_k8s.azurerm_kubernetes_cluster_node_pool.spot[0]", "azurerm_kubernetes_cluster_node_pool"))
		total = "237.98"
	}
	out, _ := json.Marshal(map[string]any{"totalMonthlyCost": total, "projects": []any{map[string]any{"breakdown": map[string]any{"resources": resources}}}})
	return string(out)
}
