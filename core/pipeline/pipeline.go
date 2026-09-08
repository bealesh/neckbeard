// Package pipeline holds the shared delivery model (DESIGN §11.1): one small
// internal description of the behaviors we generate — test, build/scan/push,
// per-env infra plan/apply with OIDC and environment binding — rendered to GitHub
// Actions and GitLab CI by deterministic renderers so the two can never drift.
// Golden tests verify rendering; execution behavior is the release harness's job.
package pipeline

import (
	"fmt"

	"github.com/bealesh/neckbeard/core/blueprint"
)

// Model is the resolved, provider-neutral pipeline description. Everything a
// renderer needs, nothing it must infer.
type Model struct {
	App           string
	DefaultBranch string
	Region        string
	TofuVersion   string
	Dockerfile    string   // single image per app at M1; distinct dockerfiles are refused, not guessed
	Envs          []string // deployment order; prd is gated (§11.3)
}

// CI configuration names the pipelines expect, set once by bootstrap (per env):
// GitHub repository/environment variables or GitLab CI/CD variables. Bootstrap
// (M2/M3) writes them; until then the generated pipelines degrade to validate-only
// and say so.
const (
	VarPlanRole  = "NECKBEARD_AWS_PLAN_ROLE"  // + _<ENV>
	VarApplyRole = "NECKBEARD_AWS_APPLY_ROLE" // + _<ENV>
	VarRegistry  = "NECKBEARD_REGISTRY"       // + _<ENV>
)

func Build(bp *blueprint.Blueprint) (Model, error) {
	dockerfiles := map[string]bool{}
	for _, s := range bp.Services {
		if s.Dockerfile != "" {
			dockerfiles[s.Dockerfile] = true
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
		DefaultBranch: "main",
		Region:        bp.Region,
		TofuVersion:   bp.Pins.OpenTofu,
		Dockerfile:    dockerfile,
		Envs:          envs,
	}, nil
}
