package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Foundation excludes application workloads and databases. Kubernetes clusters
// are infrastructure foundations; applications arrive later through Flux.
// Planning and applying are separate so the operator can review the saved plan.
func (r Runner) Foundation(ctx context.Context, t Target, apply bool) error {
	if err := t.Validate(t.Environment); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, name := range t.FoundationModules {
		switch name {
		case "network", "registry", "secrets", "storage":
			if allowed[name] {
				return fmt.Errorf("duplicate foundation module %q", name)
			}
			allowed[name] = true
		case "runtime_k8s":
			if t.Runtime != "kubernetes" || allowed[name] {
				return fmt.Errorf("invalid Kubernetes foundation target")
			}
			allowed[name] = true
		default:
			return fmt.Errorf("module %q is not a deployment foundation", name)
		}
	}
	if !allowed["registry"] {
		return fmt.Errorf("foundation target has no registry; re-plan and scaffold first")
	}
	dir := filepath.Join(r.Root, "infra/envs", t.Environment)
	if _, err := os.Stat(filepath.Join(dir, "backend.hcl")); err != nil {
		return fmt.Errorf("complete bootstrap and configure backend.hcl before preparing foundations: %w", err)
	}
	tofu := func(args ...string) ([]byte, error) {
		return r.command(ctx, "tofu", append([]string{"-chdir=" + dir}, args...)...)
	}
	if _, err := tofu("init", "-input=false", "-backend-config=backend.hcl"); err != nil {
		return err
	}
	if !apply {
		// Remove an older plan before trying again: a failed plan must not leave
		// an earlier successful plan available to the subsequent apply command.
		if err := os.Remove(filepath.Join(dir, "foundation.tfplan")); err != nil && !os.IsNotExist(err) {
			return err
		}
		args := []string{"plan", "-input=false", "-out=foundation.tfplan"}
		for _, name := range t.FoundationModules {
			args = append(args, "-target=module."+name)
		}
		out, err := tofu(args...)
		if err != nil {
			return err
		}
		fmt.Print(string(out))
	}
	data, err := tofu("show", "-json", "foundation.tfplan")
	if err != nil {
		return err
	}
	if err := validateFoundationPlan(data, t.Cloud, allowed); err != nil {
		// This is not an approved foundation plan, even if OpenTofu produced it.
		_ = os.Remove(filepath.Join(dir, "foundation.tfplan"))
		return err
	}
	if !apply {
		fmt.Printf("Review infra/envs/%s/foundation.tfplan before running foundation-apply. Databases and application workloads are not included.\n", t.Environment)
		return nil
	}
	if _, err := tofu("apply", "-input=false", "foundation.tfplan"); err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, "foundation.tfplan"))
}

func validateFoundationPlan(data []byte, cloud string, allowed map[string]bool, extraAddresses ...string) error {
	var plan struct {
		FormatVersion   string `json:"format_version"`
		Errored         bool   `json:"errored"`
		ResourceChanges []struct {
			Address string `json:"address"`
			Mode    string `json:"mode"`
			Change  struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		} `json:"resource_changes"`
	}
	if err := json.Unmarshal(data, &plan); err != nil || plan.FormatVersion == "" || plan.Errored {
		return fmt.Errorf("cannot validate the saved foundation plan")
	}
	for _, change := range plan.ResourceChanges {
		if len(change.Change.Actions) != 1 {
			return fmt.Errorf("foundation plan cannot replace %s", change.Address)
		}
		action := change.Change.Actions[0]
		if action == "no-op" || (action == "read" && change.Mode == "data") {
			continue
		}
		owned := cloud == "azure" && change.Address == "azurerm_resource_group.this"
		for _, address := range extraAddresses {
			owned = owned || change.Address == address
		}
		for module := range allowed {
			owned = owned || strings.HasPrefix(change.Address, "module."+module+".")
		}
		if !owned || change.Mode != "managed" || action != "create" {
			return fmt.Errorf("foundation plan refuses %s on %s; review this through the full infrastructure workflow", action, change.Address)
		}
	}
	return nil
}
