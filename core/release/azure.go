package release

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (r Runner) azure(ctx context.Context, t Target, a Artifact, values map[string]json.RawMessage) error {
	group, err := outputString(values, "resource_group")
	if err != nil {
		return err
	}
	var names map[string]string
	if err = json.Unmarshal(values["service_names"], &names); err != nil {
		return fmt.Errorf("missing service_names: %w", err)
	}
	for _, s := range t.Services {
		name := names[s.Name]
		if name == "" {
			return fmt.Errorf("missing resource name for %s", s.Name)
		}
		args := []string{"containerapp"}
		if s.Kind == "cron" {
			args = append(args, "job")
		}
		base := append([]string(nil), args...)
		args = append(args, "update", "--name", name, "--resource-group", group, "--subscription", t.Container, "--image", a.Image, "--output", "json")
		if _, err = r.command(ctx, "az", args...); err != nil {
			return err
		}
		args = append(base, "show", "--name", name, "--resource-group", group, "--subscription", t.Container, "--output", "json")
		if err := waitReady(ctx, 5*time.Minute, func() error {
			data, err := r.command(ctx, "az", args...)
			if err != nil {
				return err
			}
			var doc struct {
				Properties struct {
					State    string `json:"provisioningState"`
					Template any    `json:"template"`
					Latest   string `json:"latestRevisionName"`
					Ready    string `json:"latestReadyRevisionName"`
				} `json:"properties"`
			}
			if err = json.Unmarshal(data, &doc); err != nil {
				return err
			}
			if doc.Properties.State != "Succeeded" || !hasImage(doc.Properties.Template, a.Image) {
				return fmt.Errorf("Container App %s has not converged to requested image", name)
			}
			if s.Kind != "cron" && (doc.Properties.Latest == "" || doc.Properties.Latest != doc.Properties.Ready) {
				return fmt.Errorf("Container App %s latest revision is not ready", name)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
