package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func (r Runner) aws(ctx context.Context, t Target, a Artifact, values map[string]json.RawMessage) error {
	cluster, err := outputString(values, "cluster_name")
	if err != nil {
		return err
	}
	var families map[string]string
	if err = json.Unmarshal(values["task_families"], &families); err != nil {
		return fmt.Errorf("missing task_families output: %w", err)
	}
	aws := func(args ...string) ([]byte, error) {
		return r.command(ctx, "aws", append(args, "--region", t.Region, "--output", "json")...)
	}
	for _, s := range t.Services {
		family := families[s.Name]
		if family == "" {
			return fmt.Errorf("no task family for %s", s.Name)
		}
		data, err := aws("ecs", "describe-task-definition", "--task-definition", family)
		if err != nil {
			return err
		}
		var response struct {
			TaskDefinition map[string]json.RawMessage `json:"taskDefinition"`
		}
		if err = json.Unmarshal(data, &response); err != nil {
			return err
		}
		td := response.TaskDefinition
		// AWS rejects response-only fields on RegisterTaskDefinition. An allowlist
		// preserves supported workload configuration and never sends unrelated data.
		input := map[string]json.RawMessage{}
		for _, k := range []string{"family", "taskRoleArn", "executionRoleArn", "networkMode", "containerDefinitions", "volumes", "placementConstraints", "requiresCompatibilities", "cpu", "memory", "pidMode", "ipcMode", "proxyConfiguration", "inferenceAccelerators", "ephemeralStorage", "runtimePlatform"} {
			if v, ok := td[k]; ok {
				input[k] = v
			}
		}
		var containers []map[string]json.RawMessage
		if err = json.Unmarshal(input["containerDefinitions"], &containers); err != nil {
			return err
		}
		found := false
		for _, c := range containers {
			var name string
			json.Unmarshal(c["name"], &name)
			if name == s.Name {
				c["image"], _ = json.Marshal(a.Image)
				found = true
			}
		}
		if !found {
			return fmt.Errorf("task family %s has no container %s", family, s.Name)
		}
		input["containerDefinitions"], _ = json.Marshal(containers)
		data, _ = json.Marshal(input)
		out, err := r.awsJSON(ctx, t, "ecs", "register-task-definition", data)
		if err != nil {
			return err
		}
		var registered struct {
			TaskDefinition struct {
				ARN string `json:"taskDefinitionArn"`
			} `json:"taskDefinition"`
		}
		if err = json.Unmarshal(out, &registered); err != nil {
			return err
		}
		arn := registered.TaskDefinition.ARN
		if arn == "" {
			return fmt.Errorf("AWS returned no task definition ARN")
		}
		if s.Kind == "cron" {
			var rules map[string]string
			json.Unmarshal(values["cron_rules"], &rules)
			rule := rules[s.Name]
			if rule == "" {
				return fmt.Errorf("no cron rule for %s", s.Name)
			}
			out, err = aws("events", "list-targets-by-rule", "--rule", rule)
			if err != nil {
				return err
			}
			var result struct {
				Targets []map[string]json.RawMessage `json:"Targets"`
			}
			if err = json.Unmarshal(out, &result); err != nil {
				return err
			}
			if len(result.Targets) != 1 {
				return fmt.Errorf("expected exactly one ECS target on %s", rule)
			}
			var ecs map[string]json.RawMessage
			if err = json.Unmarshal(result.Targets[0]["EcsParameters"], &ecs); err != nil {
				return err
			}
			ecs["TaskDefinitionArn"], _ = json.Marshal(arn)
			result.Targets[0]["EcsParameters"], _ = json.Marshal(ecs)
			data, _ = json.Marshal(map[string]any{"Rule": rule, "Targets": result.Targets})
			out, err = r.awsJSON(ctx, t, "events", "put-targets", data)
			if err != nil {
				return err
			}
			var put struct {
				Failed int `json:"FailedEntryCount"`
			}
			if err = json.Unmarshal(out, &put); err != nil {
				return err
			}
			if put.Failed != 0 {
				return fmt.Errorf("EventBridge rejected the release for %s", s.Name)
			}
			continue
		}
		if _, err = aws("ecs", "update-service", "--cluster", cluster, "--service", s.Name, "--task-definition", arn); err != nil {
			return err
		}
		if _, err = aws("ecs", "wait", "services-stable", "--cluster", cluster, "--services", s.Name); err != nil {
			return err
		}
		out, err = aws("ecs", "describe-services", "--cluster", cluster, "--services", s.Name)
		if err != nil {
			return err
		}
		var result struct {
			Services []struct {
				TaskDefinition string `json:"taskDefinition"`
				Running        int    `json:"runningCount"`
				Desired        int    `json:"desiredCount"`
			} `json:"services"`
		}
		if err = json.Unmarshal(out, &result); err != nil {
			return err
		}
		if len(result.Services) != 1 || result.Services[0].TaskDefinition != arn || result.Services[0].Running < 1 || result.Services[0].Running != result.Services[0].Desired {
			return fmt.Errorf("%s did not converge to requested task definition (including possible automatic rollback)", s.Name)
		}
		out, err = aws("ecs", "list-tasks", "--cluster", cluster, "--service-name", s.Name)
		if err != nil {
			return err
		}
		var tasks struct {
			ARNs []string `json:"taskArns"`
		}
		if err = json.Unmarshal(out, &tasks); err != nil {
			return err
		}
		if len(tasks.ARNs) == 0 {
			return fmt.Errorf("no running tasks for %s", s.Name)
		}
		args := append([]string{"ecs", "describe-tasks", "--cluster", cluster, "--tasks"}, tasks.ARNs...)
		out, err = aws(args...)
		if err != nil {
			return err
		}
		var actual struct {
			Failures []json.RawMessage `json:"failures"`
			Tasks    []struct {
				Containers []struct {
					Name   string `json:"name"`
					Digest string `json:"imageDigest"`
				} `json:"containers"`
			} `json:"tasks"`
		}
		if err = json.Unmarshal(out, &actual); err != nil {
			return err
		}
		if len(actual.Failures) != 0 || len(actual.Tasks) != len(tasks.ARNs) {
			return fmt.Errorf("AWS did not return every running task for %s", s.Name)
		}
		wanted := strings.Split(a.Image, "@")[1]
		for _, task := range actual.Tasks {
			matched := false
			for _, c := range task.Containers {
				if c.Name == s.Name {
					matched = c.Digest == wanted
				}
			}
			if !matched {
				return fmt.Errorf("running container %s does not report requested image digest", s.Name)
			}
		}
	}
	return nil
}

// AWS CLI reads --cli-input-json twice, which cannot work with a pipe. Use a
// private, short-lived file for task configuration. Secret values use their
// individual --secret-string stdin parameter instead (see writeDatabaseSecret).
func (r Runner) awsJSON(ctx context.Context, t Target, service, operation string, data []byte) ([]byte, error) {
	f, err := os.CreateTemp("", "neckbeard-aws-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return r.command(ctx, "aws", service, operation, "--cli-input-json", "file://"+f.Name(), "--region", t.Region, "--output", "json")
}
