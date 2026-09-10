package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Infra supplies committed release inputs and ephemeral database credentials to
// the normal infrastructure workflow. PR plans use a public placeholder for the
// write-only password; their cloud identity never needs secret-value access.
func (r Runner) Infra(ctx context.Context, t Target, apply, readOnly bool) error {
	if err := t.Validate(t.Environment); err != nil {
		return err
	}
	if apply && readOnly {
		return fmt.Errorf("read-only infrastructure plans cannot apply")
	}
	dir := filepath.Join(r.Root, "infra/envs", t.Environment)
	if _, err := os.Stat(filepath.Join(dir, "backend.hcl")); err != nil {
		return fmt.Errorf("complete bootstrap first: %w", err)
	}
	if !apply {
		if err := os.Remove(filepath.Join(dir, "tfplan")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	var environment []string
	if t.DatabaseSecret != "" {
		password := "Nb9_PlanOnlyPlaceholder"
		if !readOnly {
			var err error
			password, err = r.databasePassword(ctx, t)
			if err != nil {
				return err
			}
		}
		environment = append(environment, "TF_VAR_database_password="+password)
	}
	args := []string{"-chdir=" + dir}
	if apply {
		args = append(args, "apply", "-input=false", "tfplan")
	} else {
		args = append(args, "plan", "-input=false", "-out=tfplan")
		if readOnly {
			args = append(args, "-lock=false")
		}
		if t.Runtime == "serverless-containers" {
			_, a, err := r.Load(t.Environment)
			if err != nil {
				return fmt.Errorf("pin and commit the first application image before the full infrastructure plan: %w", err)
			}
			varsFile := filepath.Join(r.Root, "releases", t.Environment+".tfvars.json")
			var vars struct {
				Image string `json:"app_image"`
			}
			if err := ReadJSON(varsFile, &vars); err != nil {
				return err
			}
			if vars.Image != a.Image {
				return fmt.Errorf("release JSON and infrastructure image input differ; re-pin the release")
			}
			args = append(args, "-var-file="+varsFile)
		}
	}
	out, err := r.Run(ctx, "tofu", args, nil, environment...)
	if err != nil {
		return err
	}
	fmt.Print(string(out))
	return nil
}
