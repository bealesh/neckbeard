package release

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (r Runner) gcp(ctx context.Context, t Target, a Artifact, values map[string]json.RawMessage) error {
	var names map[string]string
	if err := json.Unmarshal(values["service_names"], &names); err != nil {
		return fmt.Errorf("missing service_names: %w", err)
	}
	for _, s := range t.Services {
		name := names[s.Name]
		if name == "" {
			return fmt.Errorf("missing resource name for %s", s.Name)
		}
		kind := map[string]string{"http": "services", "worker": "worker-pools", "cron": "jobs"}[s.Kind]
		if kind == "" {
			return fmt.Errorf("unsupported service kind %s", s.Kind)
		}
		args := []string{"run", kind, "update", name, "--image", a.Image, "--region", t.Region, "--project", t.Container, "--format=json", "--quiet"}
		if _, err := r.command(ctx, "gcloud", args...); err != nil {
			return err
		}
		err := waitReady(ctx, 5*time.Minute, func() error {
			data, err := r.command(ctx, "gcloud", "run", kind, "describe", name, "--region", t.Region, "--project", t.Container, "--format=json")
			if err != nil {
				return err
			}
			var doc map[string]any
			if err = json.Unmarshal(data, &doc); err != nil {
				return err
			}
			return cloudRunReady(doc, s.Kind, a.Image)
		})
		if err != nil {
			return fmt.Errorf("Cloud Run %s: %w", name, err)
		}
	}
	return nil
}
func hasImage(v any, image string) bool {
	switch x := v.(type) {
	case map[string]any:
		if x["image"] == image {
			return true
		}
		for _, child := range x {
			if hasImage(child, image) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if hasImage(child, image) {
				return true
			}
		}
	}
	return false
}

// Check the desired template and observed status independently; an old Ready
// condition or an image mentioned only in metadata is not deployment evidence.
func cloudRunReady(doc map[string]any, kind, image string) error {
	spec, v1 := doc["spec"].(map[string]any)
	status := doc
	generation := doc["generation"]
	if v1 {
		status, _ = doc["status"].(map[string]any)
		meta, _ := doc["metadata"].(map[string]any)
		generation = meta["generation"]
	} else {
		spec = doc
	}
	if !hasImage(spec["template"], image) {
		return fmt.Errorf("template does not contain the requested image")
	}
	if generation == nil || fmt.Sprint(generation) != fmt.Sprint(status["observedGeneration"]) {
		return fmt.Errorf("latest generation has not been observed")
	}
	ready := false
	if v1 {
		conditions, _ := status["conditions"].([]any)
		for _, c := range conditions {
			condition, _ := c.(map[string]any)
			if condition["type"] == "Ready" {
				ready = condition["status"] == "True"
			}
		}
	} else {
		condition, _ := status["terminalCondition"].(map[string]any)
		ready = condition["state"] == "CONDITION_SUCCEEDED" && status["reconciling"] != true
	}
	if !ready {
		return fmt.Errorf("resource is not Ready")
	}
	if kind == "cron" {
		return nil
	}
	latest, ok := status["latestCreatedRevisionName"].(string)
	actual := status["latestReadyRevisionName"]
	if !v1 {
		latest, ok = status["latestCreatedRevision"].(string)
		actual = status["latestReadyRevision"]
	}
	if !ok || latest == "" || latest != actual {
		return fmt.Errorf("latest revision is not ready")
	}
	if kind == "http" {
		traffic, _ := status["traffic"].([]any)
		if !v1 {
			traffic, _ = status["trafficStatuses"].([]any)
		}
		total := float64(0)
		for _, item := range traffic {
			entry, _ := item.(map[string]any)
			percent, _ := entry["percent"].(float64)
			revision := entry["revisionName"]
			if !v1 {
				revision = entry["revision"]
			}
			if percent > 0 && revision != actual {
				return fmt.Errorf("traffic still reaches a different revision")
			}
			total += percent
		}
		if total != 100 {
			return fmt.Errorf("requested revision does not receive all traffic")
		}
	}
	return nil
}
