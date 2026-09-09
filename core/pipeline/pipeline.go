// Package pipeline holds the shared delivery model (DESIGN §11.1): one small
// internal description of the behaviors we generate — test, build/scan/push,
// per-env infra plan/apply with OIDC and environment binding — rendered to GitHub
// Actions and GitLab CI by deterministic renderers so the two can never drift.
// Golden tests verify rendering; execution behavior is the release harness's job.
package pipeline

import (
	"fmt"
	"path"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
)

// Model is the resolved, provider-neutral pipeline description. Everything a
// renderer needs, nothing it must infer.
type Model struct {
	App           string
	Cloud         string // drives the federation and registry-login steps
	DefaultBranch string
	Region        string
	TofuVersion   string
	Dockerfile    string   // single image per app at M1; distinct dockerfiles are refused, not guessed
	Envs          []string // deployment order; prd is gated (§11.3)
}

// CI configuration names the pipelines expect, set once by bootstrap (per env):
// GitHub repository/environment variables or GitLab CI/CD variables (the bootstrap
// modules' ci_variables outputs are exactly these). Until they exist the generated
// pipelines degrade to validate-only and say so.
const (
	VarPlanRole  = "NECKBEARD_AWS_PLAN_ROLE"  // + _<ENV>
	VarApplyRole = "NECKBEARD_AWS_APPLY_ROLE" // + _<ENV>
	VarRegistry  = "NECKBEARD_REGISTRY"       // + _<ENV>

	VarGCPProvider = "NECKBEARD_GCP_WIF_PROVIDER" // + _<ENV>
	VarGCPPlanSA   = "NECKBEARD_GCP_PLAN_SA"      // + _<ENV>
	VarGCPApplySA  = "NECKBEARD_GCP_APPLY_SA"     // + _<ENV>

	VarAzureTenant      = "NECKBEARD_AZURE_TENANT_ID"
	VarAzureSub         = "NECKBEARD_AZURE_SUBSCRIPTION" // + _<ENV>
	VarAzurePlanClient  = "NECKBEARD_AZURE_PLAN_CLIENT"  // + _<ENV>
	VarAzureApplyClient = "NECKBEARD_AZURE_APPLY_CLIENT" // + _<ENV>
)

// GateVar is the variable whose absence means "bootstrap pending" for a cloud.
func GateVar(cloud, env string) string {
	e := strings.ToUpper(env)
	switch cloud {
	case "gcp":
		return VarGCPProvider + "_" + e
	case "azure":
		return VarAzurePlanClient + "_" + e
	default:
		return VarPlanRole + "_" + e
	}
}

func Build(bp *blueprint.Blueprint) (Model, error) {
	dockerfiles := map[string]bool{}
	for _, s := range bp.Services {
		if s.Dockerfile != "" {
			dockerfiles[path.Clean(s.Dockerfile)] = true
		}
	}
	if len(dockerfiles) == 0 {
		return Model{}, fmt.Errorf("no service declares a dockerfile in the app profile; the build job needs one")
	}
	if len(dockerfiles) > 1 {
		return Model{}, fmt.Errorf("services declare %d distinct dockerfiles; multi-image apps are not supported yet (M1 builds one image per app) — this is a named limitation, not a silent guess", len(dockerfiles))
	}
	var dockerfile string
	for d := range dockerfiles {
		dockerfile = d
	}

	envs := make([]string, 0, len(bp.Environments))
	for _, e := range bp.Environments {
		envs = append(envs, e.Name)
	}
	return Model{
		App:           bp.App,
		Cloud:         bp.Cloud,
		DefaultBranch: "main",
		Region:        bp.Region,
		TofuVersion:   bp.Pins.OpenTofu,
		Dockerfile:    dockerfile,
		Envs:          envs,
	}, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
