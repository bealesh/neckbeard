package release

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFoundationPlanBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, address, mode string
		actions             []string
		ok                  bool
	}{
		{"registry", "module.registry.aws_ecr_repository.this", "managed", []string{"create"}, true},
		{"dependency", "azurerm_resource_group.this", "managed", []string{"create"}, true},
		{"read", "data.aws_caller_identity.current", "data", []string{"read"}, true},
		{"workload", "module.runtime_serverless.aws_ecs_service.this", "managed", []string{"create"}, false},
		{"database", "module.postgres.aws_db_instance.this", "managed", []string{"create"}, false},
		{"update", "module.registry.aws_ecr_repository.this", "managed", []string{"update"}, false},
		{"destroy", "module.registry.aws_ecr_repository.this", "managed", []string{"delete"}, false},
		{"replace", "module.registry.aws_ecr_repository.this", "managed", []string{"delete", "create"}, false},
		{"prefix", "module.registry_extra.aws_ecr_repository.this", "managed", []string{"create"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{"format_version": "1.2", "resource_changes": []any{map[string]any{"address": tc.address, "mode": tc.mode, "change": map[string]any{"actions": tc.actions}}}})
			if err := validateFoundationPlan(data, "azure", map[string]bool{"registry": true}); (err == nil) != tc.ok {
				t.Fatalf("accepted=%v, want %v: %v", err == nil, tc.ok, err)
			}
		})
	}
	for _, data := range []string{`{}`, `{"format_version":"1.2","errored":true}`, `garbage`} {
		if validateFoundationPlan([]byte(data), "aws", nil) == nil {
			t.Fatalf("accepted invalid plan %s", data)
		}
	}
}

func TestFoundationReviewAndFailure(t *testing.T) {
	for _, mode := range []string{"plan", "apply", "failed-plan", "unsafe-apply", "invalid-target"} {
		t.Run(mode, func(t *testing.T) {
			r, target, _ := fixture(t, "aws", "worker")
			target.FoundationModules = []string{"network", "registry", "secrets"}
			dir := filepath.Join(r.Root, "infra/envs/dev")
			os.MkdirAll(dir, 0755)
			os.WriteFile(filepath.Join(dir, "backend.hcl"), nil, 0600)
			plan := filepath.Join(dir, "foundation.tfplan")
			os.WriteFile(plan, []byte("older-plan"), 0600)
			applied, planned := false, false
			if mode == "invalid-target" {
				target.FoundationModules = append(target.FoundationModules, "runtime-serverless")
			}
			r.Run = func(_ context.Context, command string, args []string, _ []byte, _ ...string) ([]byte, error) {
				if mode == "invalid-target" {
					t.Fatal("ran command for invalid target")
				}
				if command != "tofu" || args[0] != "-chdir="+dir {
					t.Fatalf("unexpected command: %s %v", command, args)
				}
				switch args[1] {
				case "init":
					return nil, nil
				case "plan":
					planned = true
					if _, err := os.Stat(plan); !os.IsNotExist(err) {
						t.Fatal("stale plan survived")
					}
					if mode == "failed-plan" {
						return nil, errors.New("quota")
					}
					if !strings.Contains(strings.Join(args, " "), "-target=module.registry") {
						t.Fatal("missing registry target")
					}
					return nil, os.WriteFile(plan, []byte("reviewable-plan"), 0600)
				case "show":
					if mode == "unsafe-apply" {
						return []byte(`{"format_version":"1.2","resource_changes":[{"address":"module.postgres.aws_db_instance.this","mode":"managed","change":{"actions":["delete"]}}]}`), nil
					}
					return []byte(`{"format_version":"1.2","resource_changes":[]}`), nil
				case "apply":
					applied = true
					return nil, nil
				default:
					t.Fatalf("unexpected tofu action %v", args)
					return nil, nil
				}
			}
			err := r.Foundation(context.Background(), target, strings.Contains(mode, "apply"))
			wantError := mode == "failed-plan" || mode == "unsafe-apply" || mode == "invalid-target"
			if (err != nil) != wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if applied != (mode == "apply") {
				t.Fatalf("applied=%v in %s", applied, mode)
			}
			if planned != (mode == "plan" || mode == "failed-plan") {
				t.Fatalf("planned=%v in %s", planned, mode)
			}
			if mode != "plan" && mode != "invalid-target" {
				if _, err := os.Stat(plan); !os.IsNotExist(err) {
					t.Fatal("stale or consumed plan remains")
				}
			}
		})
	}
}
