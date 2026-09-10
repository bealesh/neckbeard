// Package render turns a blueprint into a write-set: a pure function of the
// blueprint, byte-identical on every call (DESIGN §9).
//
// Lane wiring (which module outputs feed which module inputs, per cloud/runtime)
// is deterministic code in this package, never agent judgment. Unsupported lanes
// are refused by name — never rendered as guesses. Meaningful per-cloud
// differences (§3.3) show up here as different wiring, not hidden equivalences.
package render

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	catalogassets "github.com/bealesh/neckbeard/catalog"
	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/ownership"
	"github.com/bealesh/neckbeard/core/pipeline"
)

type Options struct {
	// CatalogSource defaults to the bundled catalog. Overrides accept a git base
	// ("git::https://…/neckbeard.git", pinned to ref=catalog-v<version>) or a local
	// path for development and tests.
	CatalogSource string
}

// DefaultCatalogSource renders the exact catalog bundled with this binary.
const DefaultCatalogSource = "bundled"

type kv struct{ k, v string }

// lane holds the deterministic wiring for one (cloud, runtime) pair.
type lane struct {
	emitOrder []string
	wiring    map[string][]kv
	// requires maps expression fragments to the module that must be present for a
	// wiring line to be emitted.
	requires map[string]string
	// rootResources are raw HCL resources owned by the env root itself (e.g. the
	// Azure resource group everything else lands in).
	rootResources func(bp *blueprint.Blueprint, env blueprint.Environment) string
	providers     func(bp *blueprint.Blueprint, env blueprint.Environment) []byte
	outputs       []rootOutput
	// delivery renders the runtime's delivery layer (e.g. clusters/ + Flux for
	// kubernetes lanes); nil for lanes where CI applies releases directly.
	delivery func(bp *blueprint.Blueprint) []ownership.File
}

type rootOutput struct{ name, module, expr, desc string }

var lanes = map[string]lane{
	"aws/serverless-containers": {
		emitOrder: []string{"network", "dns-ingress", "runtime-serverless", "postgres", "storage", "secrets", "registry"},
		wiring: map[string][]kv{
			"dns-ingress": {
				{"certificate_arn", "var.certificate_arn"},
				{"vpc_id", "module.network.vpc_id"},
				{"public_subnet_ids", "module.network.public_subnet_ids"},
				{"http_services", `[for s in local.services : { name = s.name, port = s.port, health_path = s.health_path } if s.kind == "http"]`},
			},
			"runtime-serverless": {
				{"services", "local.services"},
				{"vpc_id", "module.network.vpc_id"},
				{"private_subnet_ids", "module.network.private_subnet_ids"},
				{"alb_security_group_id", "module.dns_ingress.alb_security_group_id"},
				{"target_group_arns", "module.dns_ingress.target_group_arns"},
				{"secret_arns", "module.secrets.secret_arns"},
			},
			"postgres": {
				{"vpc_id", "module.network.vpc_id"},
				{"private_subnet_ids", "module.network.private_subnet_ids"},
				{"allowed_security_group_ids", "[module.runtime_serverless.service_security_group_id]"},
			},
		},
		requires:  stdRequires,
		providers: awsProviders,
		outputs: []rootOutput{
			{"alb_dns_name", "dns-ingress", "module.dns_ingress.alb_dns_name", "Public entry point (HTTP, M1)"},
			{"cluster_name", "runtime-serverless", "module.runtime_serverless.cluster_name", "ECS cluster running the services"},
			{"db_endpoint", "postgres", "module.postgres.endpoint", "PostgreSQL endpoint (credentials: application connection secret)"},
			{"db_master_user_secret_arn", "postgres", "module.postgres.master_user_secret_arn", "RDS-managed master secret, when managed rotation is enabled"},
			{"bucket_name", "storage", "module.storage.bucket_name", "Application object storage"},
			{"registry_url", "registry", "module.registry.repository_url", "Container registry (immutable tags)"},
		},
	},
	"aws/kubernetes": {
		// No dns-ingress module: kubernetes ingress is in-cluster
		// (aws-load-balancer-controller via Flux), delivered with the clusters/
		// layer. This lane renders the platform: cluster, data services, network.
		emitOrder: []string{"network", "runtime-k8s", "postgres", "storage", "secrets", "registry"},
		wiring: map[string][]kv{
			"runtime-k8s": {
				{"vpc_id", "module.network.vpc_id"},
				{"private_subnet_ids", "module.network.private_subnet_ids"},
			},
			"postgres": {
				{"vpc_id", "module.network.vpc_id"},
				{"private_subnet_ids", "module.network.private_subnet_ids"},
				{"allowed_security_group_ids", "[module.runtime_k8s.cluster_security_group_id]"},
			},
		},
		requires:  stdRequires,
		providers: awsProviders,
		outputs: []rootOutput{
			{"cluster_name", "runtime-k8s", "module.runtime_k8s.cluster_name", "EKS cluster (delivery via Flux lands with the clusters layer)"},
			{"cluster_endpoint", "runtime-k8s", "module.runtime_k8s.cluster_endpoint", "EKS API endpoint (public at M2; origin lockdown is a hardening roadmap item)"},
			{"oidc_issuer", "runtime-k8s", "module.runtime_k8s.oidc_issuer", "Cluster OIDC issuer for workload identity"},
			{"db_endpoint", "postgres", "module.postgres.endpoint", "PostgreSQL endpoint (credentials: application connection secret)"},
			{"db_master_user_secret_arn", "postgres", "module.postgres.master_user_secret_arn", "RDS-managed master secret, when managed rotation is enabled"},
			{"bucket_name", "storage", "module.storage.bucket_name", "Application object storage"},
			{"registry_url", "registry", "module.registry.repository_url", "Container registry (immutable tags)"},
		},
		delivery: k8sDeliveryAWS,
	},
	"gcp/kubernetes": {
		// GKE's built-in ingress class handles http; external-secrets via Workload
		// Identity handles the app secret. Both wired in the delivery layer.
		emitOrder: []string{"network", "runtime-k8s", "postgres", "storage", "secrets", "registry"},
		wiring: map[string][]kv{
			"runtime-k8s": {
				{"network_id", "module.network.network_id"},
				{"subnet_id", "module.network.subnet_id"},
				{"secret_ids", "module.secrets.secret_ids"},
			},
			"postgres": {
				{"network_id", "module.network.network_id"},
				{"private_services_connection", "module.network.private_services_connection"},
			},
		},
		requires:  stdRequires,
		providers: gcpProviders,
		outputs: []rootOutput{
			{"cluster_name", "runtime-k8s", "module.runtime_k8s.cluster_name", "GKE cluster (delivery via Flux lands with the clusters layer)"},
			{"cluster_endpoint", "runtime-k8s", "module.runtime_k8s.cluster_endpoint", "GKE API endpoint (public at M2; origin lockdown is a hardening roadmap item)"},
			{"db_connection_name", "postgres", "module.postgres.connection_name", "Cloud SQL connection name (credentials: operator-set secret)"},
			{"bucket_name", "storage", "module.storage.bucket_name", "Application object storage"},
			{"registry_url", "registry", "module.registry.repository_url", "Artifact Registry repository"},
		},
		delivery: k8sDeliveryGCP,
	},
	"gcp/serverless-containers": {
		emitOrder: []string{"network", "runtime-serverless", "dns-ingress", "postgres", "storage", "secrets", "registry"},
		wiring: map[string][]kv{
			"runtime-serverless": {
				{"services", "local.services"},
				{"subnet_id", "module.network.subnet_id"},
				{"secret_ids", "module.secrets.secret_ids"},
			},
			"dns-ingress": {
				{"hostname", "var.public_hostname"},
				{"http_services", `[for s in local.services : { name = s.name, port = s.port, health_path = s.health_path } if s.kind == "http"]`},
				{"service_names", "module.runtime_serverless.service_names"},
			},
			"postgres": {
				{"network_id", "module.network.network_id"},
				{"private_services_connection", "module.network.private_services_connection"},
			},
		},
		requires:  stdRequires,
		providers: gcpProviders,
		outputs: []rootOutput{
			{"lb_ip_address", "dns-ingress", "module.dns_ingress.lb_ip_address", "Public entry point (HTTP, M1)"},
			{"service_urls", "runtime-serverless", "module.runtime_serverless.service_urls", "Cloud Run service URLs (direct, pre-LB)"},
			{"db_connection_name", "postgres", "module.postgres.connection_name", "Cloud SQL connection name (credentials: operator-set secret)"},
			{"bucket_name", "storage", "module.storage.bucket_name", "Application object storage"},
			{"registry_url", "registry", "module.registry.repository_url", "Artifact Registry repository"},
		},
	},
	"azure/kubernetes": {
		// AKS app-routing addon handles ingress; external-secrets over workload
		// identity handles the app secret (client id lands via Flux postBuild
		// substitution from the bootstrap-published ConfigMap).
		emitOrder: []string{"network", "runtime-k8s", "postgres", "storage", "secrets", "registry"},
		wiring: map[string][]kv{
			"network": {
				{"resource_group_name", "azurerm_resource_group.this.name"},
				{"runtime", `"kubernetes"`},
			},
			"runtime-k8s": {
				{"resource_group_name", "azurerm_resource_group.this.name"},
				{"subnet_id", "module.network.aks_subnet_id"},
				{"key_vault_id", "module.secrets.key_vault_id"},
				{"registry_id", "module.registry.registry_id"},
			},
			"postgres": {
				{"resource_group_name", "azurerm_resource_group.this.name"},
				{"delegated_subnet_id", "module.network.db_subnet_id"},
				{"private_dns_zone_id", "module.network.postgres_dns_zone_id"},
			},
			"storage":  {{"resource_group_name", "azurerm_resource_group.this.name"}},
			"secrets":  {{"resource_group_name", "azurerm_resource_group.this.name"}},
			"registry": {{"resource_group_name", "azurerm_resource_group.this.name"}},
		},
		requires:      stdRequires,
		providers:     azureProviders,
		rootResources: azureResourceGroup,
		outputs: []rootOutput{
			{"cluster_name", "runtime-k8s", "module.runtime_k8s.cluster_name", "AKS cluster (delivery via Flux lands with the clusters layer)"},
			{"cluster_endpoint", "runtime-k8s", "module.runtime_k8s.cluster_endpoint", "AKS API endpoint (public at M2; origin lockdown is a hardening roadmap item)"},
			{"external_secrets_client_id", "runtime-k8s", "module.runtime_k8s.external_secrets_client_id", "Bootstrap publishes this into neckbeard-cluster-vars for Flux substitution"},
			{"db_fqdn", "postgres", "module.postgres.fqdn", "PostgreSQL flexible server FQDN (credentials: operator-set secret)"},
			{"storage_account", "storage", "module.storage.account_name", "Application object storage account"},
			{"registry_url", "registry", "module.registry.login_server", "Container registry"},
		},
		delivery: k8sDeliveryAzure,
	},
	"azure/serverless-containers": {
		// No dns-ingress module: Container Apps provides HTTPS ingress natively —
		// an honest per-cloud difference (§3.3), declared in the catalog's azure
		// capability override.
		emitOrder: []string{"network", "runtime-serverless", "postgres", "storage", "secrets", "registry"},
		wiring: map[string][]kv{
			"network": {
				{"resource_group_name", "azurerm_resource_group.this.name"},
			},
			"runtime-serverless": {
				{"services", "local.services"},
				{"resource_group_name", "azurerm_resource_group.this.name"},
				{"subnet_id", "module.network.app_subnet_id"},
				{"key_vault_id", "module.secrets.key_vault_id"},
				{"secret_uris", "module.secrets.secret_uris"},
				{"registry_server", "module.registry.login_server"},
				{"registry_id", "module.registry.registry_id"},
			},
			"postgres": {
				{"resource_group_name", "azurerm_resource_group.this.name"},
				{"delegated_subnet_id", "module.network.db_subnet_id"},
				{"private_dns_zone_id", "module.network.postgres_dns_zone_id"},
			},
			"storage":  {{"resource_group_name", "azurerm_resource_group.this.name"}},
			"secrets":  {{"resource_group_name", "azurerm_resource_group.this.name"}},
			"registry": {{"resource_group_name", "azurerm_resource_group.this.name"}},
		},
		requires:      stdRequires,
		providers:     azureProviders,
		rootResources: azureResourceGroup,
		outputs: []rootOutput{
			{"service_fqdns", "runtime-serverless", "module.runtime_serverless.service_fqdns", "Container Apps ingress FQDNs (built-in HTTPS)"},
			{"db_fqdn", "postgres", "module.postgres.fqdn", "PostgreSQL flexible server FQDN (credentials: operator-set secret)"},
			{"storage_account", "storage", "module.storage.account_name", "Application object storage account"},
			{"registry_url", "registry", "module.registry.login_server", "Container registry"},
		},
	},
}

var stdRequires = map[string]string{
	"module.network.":            "network",
	"module.dns_ingress.":        "dns-ingress",
	"module.runtime_serverless.": "runtime-serverless",
	"module.runtime_k8s.":        "runtime-k8s",
	"module.registry.":           "registry",
	"module.secrets.":            "secrets",
	"module.postgres.":           "postgres",
	"local.services":             "runtime-serverless",
}

// WriteSet renders the blueprint into files for ownership.Apply.
func WriteSet(bp *blueprint.Blueprint, opts Options) ([]ownership.File, error) {
	if opts.CatalogSource == "" {
		opts.CatalogSource = DefaultCatalogSource
	}
	if opts.CatalogSource == DefaultCatalogSource && bp.Pins.CatalogDigest != catalogassets.Digest() {
		return nil, fmt.Errorf("blueprint catalog bytes do not match this CLI's bundled catalog; re-run plan with this CLI, or supply -catalog-source for an explicit external catalog")
	}
	laneKey := bp.Cloud + "/" + bp.Runtime
	l, ok := lanes[laneKey]
	if !ok {
		supported := make([]string, 0, len(lanes))
		for k := range lanes {
			supported = append(supported, k)
		}
		sort.Strings(supported)
		return nil, fmt.Errorf("rendering for lane %s is not implemented yet (supported: %s) — the planner accepted your blueprint and no files were written", laneKey, strings.Join(supported, ", "))
	}

	model, err := pipeline.Build(bp)
	if err != nil {
		return nil, err
	}

	files := []ownership.File{
		{Path: "docs/topology.md", Content: topologyDoc(bp), Owner: ownership.OwnerGenerated},
		{Path: ".neckbeard/hooks/test.sh", Content: testHookStub(), Owner: ownership.OwnerUser, Mode: 0o755},
		{Path: ".checkov.yaml", Content: checkovConfig(bp.Cloud), Owner: ownership.OwnerGenerated},
	}
	if opts.CatalogSource == DefaultCatalogSource {
		err := fs.WalkDir(catalogassets.Files, ".", func(p string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			data, err := catalogassets.Files.ReadFile(p)
			if err != nil {
				return err
			}
			files = append(files, ownership.File{Path: bundleDir(bp) + "/catalog/" + p, Content: data, Owner: ownership.OwnerGenerated})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	switch bp.VCS {
	case "github":
		files = append(files,
			ownership.File{Path: ".github/workflows/neckbeard-ci.yml", Content: pipeline.RenderGitHubCI(model), Owner: ownership.OwnerGenerated},
			ownership.File{Path: ".github/workflows/neckbeard-infra.yml", Content: pipeline.RenderGitHubInfra(model), Owner: ownership.OwnerGenerated},
			ownership.File{Path: ".github/workflows/neckbeard-release.yml", Content: pipeline.RenderGitHubRelease(model), Owner: ownership.OwnerGenerated},
		)
	case "gitlab":
		files = append(files,
			ownership.File{Path: ".gitlab-ci.yml", Content: pipeline.RenderGitLab(model), Owner: ownership.OwnerGenerated},
		)
	default:
		return nil, fmt.Errorf("unknown vcs %q in blueprint", bp.VCS)
	}
	for _, env := range bp.Environments {
		dir := "infra/envs/" + env.Name
		files = append(files,
			ownership.File{Path: dir + "/backend.tf", Content: backendTF(bp.Cloud), Owner: ownership.OwnerGenerated},
			ownership.File{Path: dir + "/providers.tf", Content: l.providers(bp, env), Owner: ownership.OwnerGenerated},
			ownership.File{Path: dir + "/main.tf", Content: mainTF(bp, env, l, opts), Owner: ownership.OwnerGenerated},
			ownership.File{Path: dir + "/outputs.tf", Content: outputsTF(bp, env, l), Owner: ownership.OwnerGenerated},
			ownership.File{Path: dir + "/custom.tf", Content: customTFStub(env.Name), Owner: ownership.OwnerUser},
		)
	}
	files = append(files, bootstrapFiles(bp, l, opts)...)
	files = append(files, releaseFiles(bp)...)
	if l.delivery != nil {
		files = append(files, l.delivery(bp)...)
	}
	return files, nil
}

const generatedHeader = "# Generated by neckbeard — do not edit. Regeneration refuses modified files;\n# change neckbeard.yaml or app-profile.yaml and re-run plan + scaffold.\n# Additions belong in custom.tf (user-owned, never regenerated).\n\n"

func backendTF(cloud string) []byte {
	backend := map[string]string{"aws": "s3", "gcp": "gcs", "azure": "azurerm"}[cloud]
	return fmt.Appendf(nil, generatedHeader+`terraform {
  # Partial backend configuration: the concrete settings are supplied at init time
  # from bootstrap outputs (M2/M3): tofu init -backend-config=…
  # V0 static validation initializes with -backend=false.
  backend %q {}
}
`, backend)
}

func awsProviders(bp *blueprint.Blueprint, env blueprint.Environment) []byte {
	var b strings.Builder
	b.WriteString(generatedHeader)
	b.WriteString(requiredProviders(bp))
	fmt.Fprintf(&b, `provider "aws" {
  region = %q

  default_tags {
    tags = {
      "neckbeard-app" = %q
      "neckbeard-env" = %q
      "managed-by"    = "neckbeard"
    }
  }
}
`, bp.Region, bp.App, env.Name)
	return []byte(b.String())
}

func gcpProviders(bp *blueprint.Blueprint, env blueprint.Environment) []byte {
	var b strings.Builder
	b.WriteString(generatedHeader)
	b.WriteString(requiredProviders(bp))
	fmt.Fprintf(&b, `provider "google" {
  project = %q
  region  = %q

  default_labels = {
    neckbeard-app = %q
    neckbeard-env = %q
    managed-by    = "neckbeard"
  }
}
`, env.Container, bp.Region, bp.App, env.Name)
	return []byte(b.String())
}

func azureProviders(bp *blueprint.Blueprint, env blueprint.Environment) []byte {
	var b strings.Builder
	b.WriteString(generatedHeader)
	b.WriteString(requiredProviders(bp))
	fmt.Fprintf(&b, `provider "azurerm" {
  features {}
  storage_use_azuread = true
  subscription_id     = %q
}
`, env.Container)
	return []byte(b.String())
}

func requiredProviders(bp *blueprint.Blueprint) string {
	var b strings.Builder
	b.WriteString("terraform {\n  required_version = \">= 1.8.0\"\n\n  required_providers {\n")
	for _, p := range bp.Pins.Providers {
		name := p.Name[strings.LastIndex(p.Name, "/")+1:]
		fmt.Fprintf(&b, "    %s = {\n      source  = %q\n      version = %q\n    }\n", name, p.Name, p.Version)
	}
	b.WriteString("  }\n}\n\n")
	return b.String()
}

func azureResourceGroup(bp *blueprint.Blueprint, env blueprint.Environment) string {
	return fmt.Sprintf(`resource "azurerm_resource_group" "this" {
  name     = %q
  location = %q

  tags = {
    neckbeard-app = %q
    neckbeard-env = %q
    managed-by    = "neckbeard"
  }
}

`, fmt.Sprintf("%s-%s-%s", bp.Org, bp.App, env.Name), bp.Region, bp.App, env.Name)
}

// mainTF wires the env's modules together per the lane tables.
func mainTF(bp *blueprint.Blueprint, env blueprint.Environment, l lane, opts Options) []byte {
	present := map[string]blueprint.ModuleUsage{}
	for _, m := range env.Modules {
		present[m.Name] = m
	}

	var b strings.Builder
	b.WriteString(generatedHeader)

	if _, ok := present["runtime-serverless"]; ok {
		b.WriteString(servicesLocal(bp.Services))
		b.WriteString("\n")
	}
	if l.rootResources != nil {
		b.WriteString(l.rootResources(bp, env))
	}

	for _, name := range l.emitOrder {
		mod, ok := present[name]
		if !ok {
			continue
		}
		lines := []kv{{"source", strconv.Quote(moduleSource(opts, bp, mod))}}
		if name == "postgres" {
			lines = append(lines, kv{"manage_app_credentials", "true"}, kv{"application_password", "var.database_password"})
		}
		if name == "runtime-serverless" {
			lines = append(lines, kv{"image", "var.app_image"}, kv{"environment", strconv.Quote(env.Name)})
		}
		for _, in := range mod.Inputs {
			lines = append(lines, kv{in.Key, hclValue(in.Value)})
		}
		for _, w := range l.wiring[name] {
			missing := false
			for prefix, dep := range l.requires {
				if strings.Contains(w.v, prefix) {
					if _, have := present[dep]; !have {
						missing = true
					}
				}
			}
			if !missing {
				lines = append(lines, w)
			}
		}
		rest := lines[1:] // keep source first, sort the assignments
		sort.SliceStable(rest, func(i, j int) bool { return rest[i].k < rest[j].k })
		fmt.Fprintf(&b, "module %q {\n%s}\n\n", hclLabel(name), alignKV(lines, 2))
	}
	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}

func servicesLocal(services []blueprint.Service) string {
	var b strings.Builder
	b.WriteString("locals {\n  services = [\n")
	for _, s := range services {
		lines := []kv{
			{"name", strconv.Quote(s.Name)},
			{"kind", strconv.Quote(s.Kind)},
			{"port", strconv.Itoa(s.Port)},
			{"health_path", strconv.Quote(s.HealthPath)},
			{"schedule", strconv.Quote(s.Schedule)},
			{"args", hclValue(s.Args)},
		}
		b.WriteString("    {\n" + alignKV(lines, 6) + "    },\n")
	}
	b.WriteString("  ]\n}\n")
	return b.String()
}

func outputsTF(bp *blueprint.Blueprint, env blueprint.Environment, l lane) []byte {
	present := map[string]bool{}
	for _, m := range env.Modules {
		present[m.Name] = true
	}
	var b strings.Builder
	b.WriteString(generatedHeader)
	for _, c := range l.outputs {
		if present[c.module] {
			fmt.Fprintf(&b, "output %q {\n  description = %q\n  value       = %s\n}\n\n", c.name, c.desc, c.expr)
		}
	}
	if present["secrets"] {
		expr := map[string]string{"aws": "module.secrets.secret_arns", "gcp": "module.secrets.secret_ids", "azure": "module.secrets.secret_uris"}[bp.Cloud]
		fmt.Fprintf(&b, "output \"secret_locations\" {\n  description = \"Secret names and locations only; values never enter infrastructure state.\"\n  value       = %s\n}\n\n", expr)
	}
	if present["postgres"] {
		host := map[string]string{"aws": "module.postgres.address", "gcp": "module.postgres.private_ip", "azure": "module.postgres.fqdn"}[bp.Cloud]
		fmt.Fprintf(&b, "output \"database\" {\n  value = {\n    host = %s\n    user = \"neckbeard\"\n    name = \"app\"\n  }\n}\n\n", host)
	}
	return []byte(strings.TrimRight(b.String(), "\n") + "\n" + deploymentOutput(bp, env))
}

func moduleSource(opts Options, bp *blueprint.Blueprint, mod blueprint.ModuleUsage) string {
	if opts.CatalogSource == DefaultCatalogSource {
		// Both env and bootstrap roots are three directories below the repo root.
		return "../../../" + bundleDir(bp) + "/" + mod.Source
	}
	if strings.HasPrefix(opts.CatalogSource, "git::") {
		return fmt.Sprintf("%s//%s?ref=catalog-v%s", opts.CatalogSource, mod.Source, bp.Pins.Catalog)
	}
	return path.Join(opts.CatalogSource, mod.Source)
}

func bundleDir(bp *blueprint.Blueprint) string {
	return ".neckbeard/catalog/" + strings.TrimPrefix(bp.Pins.CatalogDigest, "sha256:")
}

// alignKV renders `key = value` lines padded the way tofu fmt aligns a block of
// consecutive assignments, so rendered files are fmt-clean by construction.
func alignKV(lines []kv, indent int) string {
	width := 0
	for _, l := range lines {
		if len(l.k) > width {
			width = len(l.k)
		}
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "%s%-*s = %s\n", strings.Repeat(" ", indent), width, l.k, l.v)
	}
	return b.String()
}

func hclLabel(moduleName string) string {
	return strings.ReplaceAll(moduleName, "-", "_")
}

// HCL strings are templates; preserve literal shell expressions in command args.
func hclString(s string) string {
	return strconv.Quote(strings.NewReplacer("${", "$${", "%{", "%%{").Replace(s))
}

func hclValue(v any) string {
	switch t := v.(type) {
	case string:
		return hclString(t)
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []string:
		quoted := make([]string, len(t))
		for i, s := range t {
			quoted[i] = hclString(s)
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = hclValue(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		// The blueprint schema restricts values to scalars and string arrays; anything
		// else is a planner bug and must fail loudly at render time.
		panic(fmt.Sprintf("unrenderable blueprint value %T (%v)", v, v))
	}
}

// checkovConfig emits the policy-skip list. Every skip carries its reason: a skip
// without a reason is a lie about the security posture (DESIGN §10). Findings not
// listed here gate the pipeline.
func checkovConfig(cloud string) []byte {
	common := []kv{
		{"CKV_TF_1", "module pinning is enforced by neckbeard itself: blueprints pin catalog versions and git sources pin ref=catalog-v<version> tags; commit-hash pinning is incompatible with the catalog versioning scheme"},
		{"CKV_TF_2", "same as CKV_TF_1 — tags are pinned via the blueprint, and local paths are used in development"},
	}
	commonCloud := map[string][]kv{
		"aws": {
			{"CKV2_AWS_31", "WAF logging (firehose + logging configuration) is a log-cost decision; ships with the o11y roadmap"},
			{"CKV2_AWS_62", "S3 event notifications have no consumer in this architecture; enabling them would be decoration"},
			{"CKV_AWS_144", "cross-region replication is a multi-region control; multi-region is explicitly unsupported at launch (§3.2)"},
			{"CKV2_AWS_76", "the log4j managed rule ships in the WAF (KnownBadInputs) wherever waf_enabled is true; WAF itself is a tier preset, deliberately off at small tiers"},
			{"CKV2_AWS_11", "VPC flow logs are a log-cost decision; regulated-tier roadmap (same family as the GCP flow-log skips)"},
			{"CKV2_AWS_57", "secret VALUES are operator-managed out-of-band by design (§8); rotation automation needs a consumer-aware rotator — roadmap"},
			{"CKV_AWS_18", "S3 access logging is a log-cost decision (an access-log bucket for the log bucket); o11y roadmap"},
			{"CKV_AWS_103", "the M1 listener is HTTP :80 by design (same decision as CKV_AWS_2/260); TLS lands with the environment manifest"},
			{"CKV_AWS_378", "same M1 HTTP decision as CKV_AWS_2/260/103"},
			{"CKV2_AWS_20", "same M1 HTTP decision: the HTTPS redirect arrives with TLS itself (environment manifest)"},
			{"CKV2_AWS_30", "postgres statement/query logging is a log-cost decision, deliberately consistent with the GCP flags family (CKV2_GCP_13, CKV_GCP_111)"},
			{"CKV2_AWS_5", "false positive: the endpoints security group IS attached — via aws_vpc_endpoint.security_group_ids, which checkov's attachment graph does not model"},
		},
		"gcp": {
			{"CKV2_GCP_13", "log_duration logs every statement's timing — a log-cost decision; the core postgres log flags are on (same family as CKV_GCP_108-111)"},
			{"CKV2_GCP_18", "an explicit deny-all-ingress rule is codified in the network module; checkov's graph check still wants allow-rule pairs the architecture doesn't need"},
		},
		"azure": {
			{"CKV2_AZURE_31", "subnets are private and workloads carry platform-level controls (CA env / AKS network policy); per-subnet NSGs are hardening roadmap"},
			{"CKV2_AZURE_21", "blob read-logging is a log-cost decision; storage analytics ships with the o11y roadmap"},
			{"CKV2_AZURE_1", "customer-managed keys are regulated-tier roadmap; platform-managed encryption is on"},
			{"CKV2_AZURE_33", "storage private endpoints are post-M1, same family as the Key Vault endpoint decision"},
			{"CKV2_AZURE_41", "SAS cannot exist here: shared_access_key_enabled is false (Entra-only data plane), so an expiration policy has nothing to govern"},
			{"CKV2_AZURE_57", "the flexible server uses VNet-injected private access (delegated subnet + private DNS, no public endpoint); checkov's check only recognizes the private-endpoint flavor of private"},
			{"CKV2_AZURE_32", "Key Vault private endpoints are post-M1, same decision as CKV_AZURE_109/189 (the vault's public endpoint stays RBAC-gated until then)"},
		},
	}
	common = append(common, commonCloud[cloud]...)
	perCloud := map[string][]kv{
		"aws": {
			{"CKV_AWS_2", "M1 ingress is HTTP :80 by design; TLS + custom domains land with the environment manifest (M2) — documented in the topology doc"},
			{"CKV_AWS_37", "EKS control-plane logging is enabled for api + audit; the full set (authenticator/controllerManager/scheduler) is a log-cost decision, regulated-tier roadmap"},
			{"CKV_AWS_38", "the EKS public endpoint is documented M2 posture (private access is also enabled); origin allowlisting/private-only requires in-VPC CI runners or VPN — hardening roadmap, stated in the module and outputs"},
			{"CKV_AWS_39", "same as CKV_AWS_38: public endpoint stays on until in-VPC access paths exist"},
			{"CKV_AWS_58", "EKS encrypts secrets at rest by default on current platform versions; customer-managed envelope KMS keys are regulated-tier roadmap"},
			{"CKV_AWS_260", "same as CKV_AWS_2: the :80 listener is the documented M1 limitation"},
			{"CKV_AWS_91", "ALB access logging (log bucket + lifecycle) is on the M2 roadmap"},
			{"CKV_AWS_118", "RDS enhanced monitoring is on the roadmap; base CloudWatch metrics + postgres log exports are on"},
			{"CKV_AWS_136", "customer-managed KMS keys are regulated-tier roadmap; AWS-managed encryption is enabled"},
			{"CKV_AWS_149", "customer-managed KMS keys are regulated-tier roadmap; AWS-managed encryption is enabled"},
			{"CKV_AWS_150", "LB deletion protection as a prd preset input is on the roadmap"},
			{"CKV_AWS_157", "multi-AZ is a tier availability dimension, deliberately off at small tiers; presets enable it where the tier's availability objective requires"},
			{"CKV_AWS_158", "customer-managed KMS keys are regulated-tier roadmap; AWS-managed encryption is enabled"},
			{"CKV_AWS_161", "RDS IAM auth is on the roadmap; the master password is RDS-managed and never in state"},
			{"CKV_AWS_293", "DB deletion protection is a per-env preset: prd enables it; dev/stg stay tear-down-able for the deploy→verify→teardown flow"},
			{"CKV_AWS_338", "30-day log retention is a tier cost decision; the estimate names log ingestion/retention as a usage assumption"},
			{"CKV_AWS_353", "performance insights is enabled from the 'small' class up; the smallest shared-core class does not support it"},
			{"CKV_AWS_354", "customer-managed KMS keys are regulated-tier roadmap; performance insights uses AWS-managed encryption"},
			{"CKV_AWS_109", "the bootstrap apply role's iam:* is constrained to the app's name-prefix ARNs — the constraint checkov wants, expressed as resource scoping; least-privilege refinement is release-harness roadmap"},
			{"CKV_AWS_356", "the wildcard statement is read-only IAM lookups (Get/List); every write is prefix-scoped"},
		},
		"gcp": {
			{"CKV_GCP_6", "TLS is enforced via ssl_mode = ENCRYPTED_ONLY; checkov still looks for the deprecated require_ssl field"},
			{"CKV_GCP_12", "NetworkPolicy is enforced natively by Dataplane V2 (datapath_provider = ADVANCED_DATAPATH); checkov looks for the legacy network_policy addon block"},
			{"CKV_GCP_20", "the GKE public endpoint is documented M2 posture (nodes are private); master authorized networks / private-only requires in-VPC CI runners or VPN — hardening roadmap, same as the EKS lane"},
			{"CKV_GCP_61", "VPC flow logs + intranode visibility are log-cost decisions; regulated-tier roadmap (consistent with CKV_GCP_26)"},
			{"CKV_GCP_65", "Google Groups RBAC is an org-level construct; lands with the founding path (M3)"},
			{"CKV_GCP_66", "Binary Authorization is supply-chain roadmap; today's gate is trivy blocking HIGH/CRITICAL in CI (§10.1)"},
			{"CKV_GCP_69", "GKE_METADATA is set on the node pool's workload_metadata_config; the default pool is removed — checkov also wants it on the cluster-level node_config that never creates nodes"},
			{"CKV_GCP_117", "the bootstrap CI identities use viewer (plan) and editor (apply) at project level; decomposing into predefined roles is release-harness roadmap, and Path A's per-env project bounds the blast radius"},
			{"CKV_GCP_49", "the apply identity manages service accounts because catalog modules create them (runtime SAs, workload identity); same refinement roadmap as CKV_GCP_117"},
			{"CKV_GCP_62", "state-bucket access logging is Cloud Audit Logs territory — landing-zone posture (M3); a log bucket for the log bucket is not the M3 slice"},
			{"CKV_GCP_26", "VPC flow logs are a log-cost decision; regulated-tier roadmap"},
			{"CKV_GCP_79", "pinned to POSTGRES_17, the current major; checkov's latest-version list lags and pinning beats floating"},
			{"CKV_GCP_84", "customer-managed encryption keys are regulated-tier roadmap; Google-managed encryption is on"},
			{"CKV_GCP_108", "verbose postgres logging (hostnames) is a log-cost decision; core log flags are on; regulated-tier roadmap"},
			{"CKV_GCP_109", "log_min_messages tuning is a log-cost decision; regulated-tier roadmap"},
			{"CKV_GCP_110", "pgAudit is regulated-tier roadmap (audit-grade logging with its cost shown in the estimate)"},
			{"CKV_GCP_111", "log_statement verbosity is a log-cost decision; regulated-tier roadmap"},
		},
		"azure": {
			{"CKV_AZURE_139", "ACR is pinned to sku Basic (cost floor); this and the ACR checks below need Premium (~10x), and image scanning is a CI gate (trivy, DESIGN §10.1), not a registry feature"},
			{"CKV_AZURE_163", "vulnerability scanning at the registry needs Defender/Premium; the supply-chain gate is trivy in CI (§10.1)"},
			{"CKV_AZURE_164", "ACR content trust needs Premium; promotion moves image DIGESTS (§11.2), immutable by construction"},
			{"CKV_AZURE_165", "geo-replication is a multi-region control; multi-region is explicitly unsupported at launch (§3.2)"},
			{"CKV_AZURE_166", "quarantine policy needs Premium; the merge gate scans images before they are ever pushed"},
			{"CKV_AZURE_167", "untagged-manifest retention policy needs Premium; registry hygiene is a roadmap item"},
			{"CKV_AZURE_233", "ACR zone redundancy needs Premium; availability objectives are tier presets, not registry defaults"},
			{"CKV_AZURE_237", "dedicated data endpoints need Premium"},
			{"CKV_AZURE_42", "Key Vault purge protection is deliberately OFF at M1: the deploy→verify→teardown release matrix (§12) needs vault names reusable after delete — see catalog/azure/secrets"},
			{"CKV_AZURE_110", "same as CKV_AZURE_42: purge protection off for teardown-ability at M1"},
			{"CKV_AZURE_109", "the vault keeps its RBAC-gated public endpoint: Container Apps Key Vault references are not a KV trusted service, so a default-deny firewall breaks secret resolution; private endpoint is post-M1"},
			{"CKV_AZURE_189", "same as CKV_AZURE_109: public network access stays on until a private endpoint lands"},
			{"CKV_AZURE_136", "geo-redundant backup is a multi-region control, unsupported at launch (§3.2), and it doubles backup cost"},
			{"CKV_AZURE_33", "queue-service logging: the queue service is unused (queues are out of the launch workload contract, §3.2) and storage analytics is a log-cost decision"},
			{"CKV_AZURE_43", "false positive: the account name is derived with replace()/substr() and adheres to the rules; checkov cannot evaluate the expression"},
			{"CKV_AZURE_59", "anonymous blob access is off (allow_nested_items_to_be_public = false); disabling the public ENDPOINT would sever Container Apps' data-plane access — private endpoints are post-M1, same as Key Vault"},
			{"CKV_AZURE_206", "LRS is the catalog cost floor; blob redundancy (ZRS/GRS) as an availability-preset input is a roadmap item and an online upgrade"},
			{"CKV_AZURE_115", "the AKS public endpoint is documented M2 posture, same as EKS/GKE; private clusters need in-VPC CI runners or VPN — hardening roadmap"},
			{"CKV_AZURE_6", "same as CKV_AZURE_115: API authorized IP ranges land with the origin-lockdown hardening work"},
			{"CKV_AZURE_116", "the Azure Policy addon is an org-guardrail construct; lands with the founding path (M3)"},
			{"CKV_AZURE_117", "disk encryption sets are customer-managed-key territory; regulated-tier roadmap (platform-managed encryption is on)"},
			{"CKV_AZURE_172", "N/A: secrets flow through external-secrets with a refreshInterval, not the Secrets Store CSI driver this check targets"},
			{"CKV_AZURE_226", "ephemeral OS disks need VM sizes with cache disks; the catalog's Dads_v5 shapes have none — revisit with the size map"},
			{"CKV_AZURE_227", "encryption-at-host requires the subscription's EncryptionAtHost feature registration — a bootstrap (M3) step; enabled then"},
			{"CKV_AZURE_232", "single-pool topologies at small tiers run workloads on the default pool by design; spot mode isolates system pods on it"},
		},
	}
	var b strings.Builder
	b.WriteString("# Generated by neckbeard — policy-skip list for checkov (V0 gate).\n")
	b.WriteString("# Every skip states its reason; anything not listed here blocks the gate.\n")
	b.WriteString("skip-check:\n")
	for _, s := range append(common, perCloud[cloud]...) {
		fmt.Fprintf(&b, "  # %s\n  - %s\n", s.v, s.k)
	}
	return []byte(b.String())
}

func testHookStub() []byte {
	return []byte(`#!/usr/bin/env sh
# .neckbeard/hooks/test.sh — user-owned test hook, run by the generated CI test job.
# neckbeard created this file once and will never regenerate it.
#
# Replace the lines below with your real test command(s), e.g.:
#   go test ./...        or        mix test        or        npm test
echo "neckbeard test hook: no tests configured yet — edit .neckbeard/hooks/test.sh"
exit 0
`)
}

func customTFStub(env string) []byte {
	return fmt.Appendf(nil, `# custom.tf — user extension point for the %s environment.
#
# neckbeard created this file once and will never regenerate or delete it.
# Resources declared here live alongside the generated stack. Prefer a catalog
# override in neckbeard.yaml for anything the catalog supports; use this file for
# what it does not.
`, env)
}

func topologyDoc(bp *blueprint.Blueprint) []byte {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("# Topology — %s\n\n", bp.App)
	w("> Generated by neckbeard from blueprint `%s`. Do not edit: regeneration will\n", bp.Hash)
	w("> refuse to overwrite a modified copy. Change `neckbeard.yaml` or\n")
	w("> `app-profile.yaml` and re-run `neckbeard plan && neckbeard scaffold` instead.\n\n")

	w("| | |\n|---|---|\n")
	w("| app | `%s` (org `%s`) |\n", bp.App, bp.Org)
	w("| cloud / region | %s / %s |\n", bp.Cloud, bp.Region)
	w("| runtime | %s |\n", bp.Runtime)
	w("| VCS | %s |\n", bp.VCS)
	w("| tier | %s |\n", bp.Tier)
	w("| pins | catalog %s · opentofu %s · planner %s |\n\n", bp.Pins.Catalog, bp.Pins.OpenTofu, bp.Pins.Planner)

	if len(bp.Services) > 0 {
		w("## Services\n\n| service | kind | port | health | schedule |\n|---|---|---|---|---|\n")
		for _, s := range bp.Services {
			w("| %s | %s | %s | %s | %s |\n", s.Name, s.Kind, orDash(s.Port), orDashS(s.HealthPath), orDashS(s.Schedule))
		}
		w("\n")
	}

	for _, env := range bp.Environments {
		w("## Environment: %s\n\n", env.Name)
		if env.Container != "" {
			w("Container: `%s`\n\n", env.Container)
		}
		w("| module | version | inputs |\n|---|---|---|\n")
		for _, m := range env.Modules {
			var parts []string
			for _, in := range m.Inputs {
				s := fmt.Sprintf("%s=%v", in.Key, in.Value)
				switch in.Provenance {
				case "override-supported":
					s += " (override)"
				case "override-warned":
					s += " (override, outside tested envelope)"
				}
				parts = append(parts, s)
			}
			w("| %s | %s | %s |\n", m.Name, m.Version, strings.Join(parts, ", "))
		}
		w("\n")
	}

	if len(bp.References) > 0 {
		w("## External references (not provisioned)\n\n")
		for _, r := range bp.References {
			w("- **%s** — existing service, consumed via secret `%s`; neckbeard does not manage its lifecycle\n", r.Capability, r.SecretName)
		}
		w("\n")
	}
	if len(bp.Warnings) > 0 {
		w("## Warnings\n\n")
		for _, warning := range bp.Warnings {
			w("- %s\n", warning)
		}
	}
	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}

func orDash(i int) string {
	if i == 0 {
		return "—"
	}
	return strconv.Itoa(i)
}

func orDashS(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
