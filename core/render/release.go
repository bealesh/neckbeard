package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/ownership"
	"github.com/bealesh/neckbeard/core/release"
)

func releaseFiles(bp *blueprint.Blueprint) []ownership.File {
	files := []ownership.File{
		{Path: ".neckbeard/install-release-tools.sh", Content: releaseTools(bp), Owner: ownership.OwnerGenerated, Mode: 0755},
		{Path: ".neckbeard/deploy/.gitignore", Content: []byte("*.receipt.json\n*.previous.json\n*.kubeconfig\n*.lock\n"), Owner: ownership.OwnerGenerated},
		{Path: "infra/.gitignore", Content: []byte("**/.terraform/\n**/*.tfstate\n**/*.tfstate.*\n**/*.tfplan\n**/tfplan\ncrash.log\ncrash.*.log\n"), Owner: ownership.OwnerGenerated},
	}
	entries, err := release.Sources.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		data, err := release.Sources.ReadFile(entry.Name())
		if err != nil {
			panic(err)
		}
		source := strings.Replace(string(data), "package release", "package main", 1)
		files = append(files, ownership.File{Path: ".neckbeard/release/" + entry.Name(), Content: []byte(source), Owner: ownership.OwnerGenerated})
	}
	files = append(files, ownership.File{Path: ".neckbeard/release/go.mod", Content: []byte("module neckbeard-release\n\ngo 1.27.1\n"), Owner: ownership.OwnerGenerated}, ownership.File{Path: ".neckbeard/release/main.go", Content: []byte("package main\nimport (\"fmt\";\"os\")\nfunc main(){if err:=Main(os.Args[1:]);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}\n"), Owner: ownership.OwnerGenerated})
	for _, env := range bp.Environments {
		for _, module := range env.Modules {
			if module.Name == "postgres" {
				files = append(files, ownership.File{Path: "infra/envs/" + env.Name + "/database-credentials.tf", Owner: ownership.OwnerGenerated, Content: []byte(generatedHeader + `variable "database_password" {
  description = "Cloud-stored database password supplied by the release operator; excluded from state and plans."
  type        = string
  sensitive   = true
  ephemeral   = true
  default     = null
}
`)})
			}
		}
		target := release.Target{App: bp.App, VCS: bp.VCS, Repo: bp.Repo, Environment: env.Name, Cloud: bp.Cloud, Region: bp.Region, Container: env.Container, Runtime: bp.Runtime, Namespace: appNS(bp)}
		target.DatabaseSecret = bp.DatabaseSecret
		for _, module := range env.Modules {
			switch module.Name {
			case "network", "registry", "secrets", "storage", "runtime-k8s":
				target.FoundationModules = append(target.FoundationModules, hclLabel(module.Name))
			}
		}
		for _, s := range bp.Services {
			target.Services = append(target.Services, release.Service{Name: s.Name, Kind: s.Kind, HealthPath: s.HealthPath})
		}
		data, err := json.MarshalIndent(target, "", "  ")
		if err != nil {
			panic(err)
		}
		files = append(files, ownership.File{Path: ".neckbeard/deploy/" + env.Name + ".json", Content: append(data, '\n'), Owner: ownership.OwnerGenerated})
		if bp.Runtime == "serverless-containers" {
			if bp.Cloud == "aws" || bp.Cloud == "gcp" {
				content := generatedHeader + `variable "public_hostname" {
  description = "Public application hostname; configure its DNS to the ingress output."
  type        = string
  default     = null
}
`
				if bp.Cloud == "aws" {
					content += `variable "certificate_arn" {
  description = "Validated ACM certificate in the deployment region."
  type        = string
  default     = null
}
`
				}
				files = append(files, ownership.File{Path: "infra/envs/" + env.Name + "/https.tf", Content: []byte(content), Owner: ownership.OwnerGenerated})
			}
			files = append(files, ownership.File{Path: "infra/envs/" + env.Name + "/release.tf", Content: []byte(generatedHeader + `variable "app_image" {
  description = "First real application image, supplied from releases/<env>.tfvars.json."
  type        = string
  default     = null
  validation {
    condition     = var.app_image == null ? true : can(regex("^.+@sha256:[0-9a-f]{64}$", var.app_image))
    error_message = "app_image must be an immutable registry image digest."
  }
}

resource "terraform_data" "release_ready" {
  lifecycle {
    precondition {
      condition     = var.app_image != null
      error_message = "No application image: prepare registry/data services, publish an image, seed secrets, then pass -var-file=../../../releases/<env>.tfvars.json. See docs/bootstrap.md."
    }
  }
}
`), Owner: ownership.OwnerGenerated})
		}
	}
	return files
}

func deploymentOutput(bp *blueprint.Blueprint, env blueprint.Environment) string {

	lines := []kv{{"registry_url", "module.registry.repository_url"}}
	if bp.Cloud == "gcp" {
		lines[0].v = fmt.Sprintf("format(\"%%s/%s\", module.registry.repository_url)", bp.App)
	}
	if bp.Cloud == "azure" {
		lines[0].v = fmt.Sprintf("format(\"%%s/%s\", module.registry.login_server)", bp.App)
	}
	if bp.Runtime == "kubernetes" {
		lines = append(lines, kv{"cluster_name", "module.runtime_k8s.cluster_name"})
		if bp.Cloud == "azure" {
			lines = append(lines, kv{"resource_group", "azurerm_resource_group.this.name"})
		}
	} else {
		switch bp.Cloud {
		case "aws":
			lines = append(lines, kv{"cluster_name", "module.runtime_serverless.cluster_name"}, kv{"task_families", "module.runtime_serverless.task_families"}, kv{"cron_rules", "module.runtime_serverless.cron_rules"})
		case "gcp":
			lines = append(lines, kv{"service_names", "module.runtime_serverless.deployment_service_names"})
		case "azure":
			lines = append(lines, kv{"service_names", "module.runtime_serverless.service_names"}, kv{"resource_group", "azurerm_resource_group.this.name"})
		}
	}
	return fmt.Sprintf("\noutput \"deployment\" {\n  description = \"Resolved resource identities used by the release runner.\"\n  value = {\n%s  }\n}\n", alignKV(lines, 4))
}
