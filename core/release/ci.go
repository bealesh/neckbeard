package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ConfigureCI publishes non-secret bootstrap and registry outputs directly to
// the VCS API. It never executes shell text produced by infrastructure outputs.
func (r Runner) ConfigureCI(ctx context.Context, t Target) error {
	if t.Repo == "" || (t.VCS != "github" && t.VCS != "gitlab") {
		return fmt.Errorf("target repository/VCS is missing")
	}
	if r.Run == nil {
		r.Run = Execute
	}
	if t.Environment == "prd" {
		if err := r.checkProductionGate(ctx, t); err != nil {
			return err
		}
	}
	if t.VCS == "github" && (t.Cloud == "aws" || t.Cloud == "azure") {
		expected, err := r.command(ctx, "tofu", "-chdir="+filepath.Join(r.Root, "infra/bootstrap", t.Environment), "output", "-raw", "github_subject_prefix")
		if err != nil {
			return err
		}
		data, err := r.command(ctx, "gh", "api", "repos/"+t.Repo+"/actions/oidc/customization/sub")
		if err != nil {
			return err
		}
		var identity struct {
			Prefix string `json:"sub_claim_prefix"`
		}
		if json.Unmarshal(data, &identity) != nil || identity.Prefix == "" || identity.Prefix != strings.TrimSpace(string(expected)) {
			return fmt.Errorf("GitHub's repository subject prefix differs from bootstrap; set github_subject_prefix from its OIDC API and reapply bootstrap before configuring CI")
		}
	}
	data, err := r.command(ctx, "tofu", "-chdir="+filepath.Join(r.Root, "infra/bootstrap", t.Environment), "output", "-json", "ci_variables")
	if err != nil {
		return err
	}
	var vars map[string]string
	if err := json.Unmarshal(data, &vars); err != nil {
		return err
	}
	data, err = r.command(ctx, "tofu", "-chdir="+filepath.Join(r.Root, "infra/bootstrap", t.Environment), "output", "-raw", "release_store")
	if err != nil {
		return err
	}
	store := ReleaseStore{URL: strings.TrimSpace(string(data))}
	if _, err := store.location(t); err != nil {
		return err
	}
	registry, err := r.Registry(ctx, t)
	if err != nil {
		return err
	}
	vars["NECKBEARD_REGISTRY_"+strings.ToUpper(t.Environment)] = registry
	keys := make([]string, 0, len(vars))
	for k := range vars {
		if !regexp.MustCompile(`^NECKBEARD_[A-Z0-9_]+$`).MatchString(k) {
			return fmt.Errorf("unexpected bootstrap variable %q", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cli := "gh"
	if t.VCS == "gitlab" {
		cli = "glab"
	}
	for _, k := range keys {
		args := []string{"variable", "set", k, "--repo", t.Repo}
		if cli == "glab" {
			args = append(args, "--raw")
		}
		if _, err := r.Run(ctx, cli, args, []byte(vars[k])); err != nil {
			return err
		}
	}
	var body any
	var args []string
	if cli == "gh" {
		body = map[string]any{"use_default": false, "include_claim_keys": []string{"repo", "environment", "ref"}}
		args = []string{"api", "repos/" + t.Repo + "/actions/oidc/customization/sub", "--method", "PUT", "--input", "-"}
	} else {
		body = map[string]any{"ci_id_token_sub_claim_components": []string{"project_path", "ref_type", "ref", "ref_protected", "deployment_tier", "environment_protected"}}
		args = []string{"api", "projects/" + url.PathEscape(t.Repo), "--method", "PUT", "--input", "-", "-H", "Content-Type: application/json"}
	}
	input, _ := json.Marshal(body)
	_, err = r.Run(ctx, cli, args, input)
	if err != nil {
		return err
	}
	return WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy", t.Environment+".store.json"), store)
}

// Refuse to publish production CI identities when the repository's approval
// feature is absent or disabled. Repository administrators must preserve this
// rule afterward; this setup check does not replace the platform's live gate.
func (r Runner) checkProductionGate(ctx context.Context, t Target) error {
	if t.VCS == "github" {
		data, err := r.command(ctx, "gh", "api", "repos/"+t.Repo+"/environments/prd")
		if err != nil {
			return fmt.Errorf("cannot verify production approval gate: %w", err)
		}
		var env struct {
			Rules []struct {
				Type        string            `json:"type"`
				PreventSelf bool              `json:"prevent_self_review"`
				Reviewers   []json.RawMessage `json:"reviewers"`
			} `json:"protection_rules"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("invalid production environment response: %w", err)
		}
		for _, rule := range env.Rules {
			if rule.Type == "required_reviewers" && rule.PreventSelf && len(rule.Reviewers) > 0 {
				return nil
			}
		}
		return fmt.Errorf("prd requires GitHub required reviewers with self-review disabled; confirm the repository's billing plan supports this before configuring production CI")
	}
	data, err := r.command(ctx, "glab", "api", "projects/"+url.PathEscape(t.Repo)+"/protected_environments/prd")
	if err != nil {
		return fmt.Errorf("cannot verify production approval gate: %w", err)
	}
	var env struct {
		Name     string `json:"name"`
		Required int    `json:"required_approval_count"`
		Rules    []struct {
			Required int `json:"required_approvals"`
		} `json:"approval_rules"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("invalid protected production environment response: %w", err)
	}
	if env.Name == "prd" {
		if env.Required > 0 {
			return nil
		}
		for _, rule := range env.Rules {
			if rule.Required > 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("prd requires a protected GitLab environment with at least one required deployment approval before configuring production CI")
}
