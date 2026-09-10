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

func TestDatabaseSetupRetryAndInfrastructureInputs(t *testing.T) {
	for _, cloud := range []string{"aws", "gcp", "azure"} {
		t.Run(cloud, func(t *testing.T) {
			r, target, artifact := fixture(t, cloud, "worker")
			target.DatabaseSecret = "DATABASE_URL"
			if cloud == "aws" {
				target.Region, target.Container = "us-east-2", "123456789012"
			}
			WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.json"), target)
			WriteJSON(filepath.Join(r.Root, "releases/dev.tfvars.json"), map[string]string{"app_image": artifact.Image})
			dir := filepath.Join(r.Root, "infra/envs/dev")
			os.MkdirAll(dir, 0755)
			os.WriteFile(filepath.Join(dir, "backend.hcl"), nil, 0600)
			location := "test-DATABASE_URL"
			if cloud == "aws" {
				location = "arn:aws:secretsmanager:us-east-2:123456789012:secret:test/DATABASE_URL-Abc123"
			}
			if cloud == "azure" {
				location = "https://test.vault.azure.net/secrets/DATABASE-URL"
			}
			secret, firstPassword := "", ""
			writes, applies := 0, 0
			failApply, denyAccess, readOnly := true, false, false
			r.Run = func(_ context.Context, command string, args []string, input []byte, environment ...string) ([]byte, error) {
				joined := strings.Join(args, " ")
				if firstPassword != "" && strings.Contains(joined, firstPassword) {
					t.Fatal("credential exposed in arguments")
				}
				if readOnly && (command != "tofu" || args[1] != "plan") {
					t.Fatalf("PR read secrets: %s %v", command, args)
				}
				if command == "tofu" {
					switch args[1] {
					case "init":
						return nil, nil
					case "output":
						if args[len(args)-1] == "secret_locations" {
							return json.Marshal(map[string]string{"DATABASE_URL": location})
						}
						return []byte(`{"host":"10.1.2.3","user":"neckbeard","name":"app"}`), nil
					case "show":
						return []byte(`{"format_version":"1.2","resource_changes":[]}`), nil
					case "plan", "apply":
						wantPassword := firstPassword
						if readOnly {
							wantPassword = "Nb9_PlanOnlyPlaceholder"
							if !strings.Contains(joined, "-lock=false") {
								t.Fatal("PR attempted state locking")
							}
						}
						if len(environment) != 1 || environment[0] != "TF_VAR_database_password="+wantPassword {
							t.Fatal("incorrect ephemeral password input")
						}
						if strings.Contains(joined, "-out=tfplan") && !strings.Contains(joined, "-var-file="+filepath.Join(r.Root, "releases/dev.tfvars.json")) {
							t.Fatal("pinned image missing from infra plan")
						}
						if args[1] == "apply" {
							applies++
							if failApply {
								return nil, errors.New("capacity")
							}
						}
						return nil, nil
					}
				}
				if command == "gcloud" || command == "az" || command == "aws" {
					if denyAccess {
						return nil, errors.New("permission denied")
					}
					if strings.Contains(joined, "describe-secret") {
						versions := map[string][]string{}
						if secret != "" {
							versions["current"] = []string{"AWSCURRENT"}
						}
						return json.Marshal(map[string]any{"ARN": location, "VersionIdsToStages": versions})
					}
					if strings.Contains(joined, "get-secret-value") {
						return json.Marshal(map[string]string{"SecretString": secret})
					}
					if strings.Contains(joined, " list ") {
						if secret == "" {
							return []byte(`[]`), nil
						}
						return []byte(`[{"name":"DATABASE-URL"}]`), nil
					}
					if strings.Contains(joined, " access ") {
						return []byte(secret), nil
					}
					if strings.Contains(joined, " show ") {
						return json.Marshal(map[string]string{"value": secret})
					}
					if strings.Contains(joined, " add ") || strings.Contains(joined, " set ") || strings.Contains(joined, "put-secret-value") {
						if command == "aws" {
							if !strings.Contains(joined, "--secret-id "+location) || !strings.Contains(joined, "--secret-string file:///dev/stdin") {
								t.Fatal("wrong secret or credential transport")
							}
						}
						secret = string(input)
						writes++
						_, password, err := parseDatabaseURL(secret)
						if err != nil {
							t.Fatal(err)
						}
						if firstPassword == "" {
							firstPassword = password
						}
						if password != firstPassword {
							t.Fatal("retry changed the password")
						}
						if command == "az" && !strings.Contains(joined, "--file /dev/stdin") {
							t.Fatal("Azure secret not passed on stdin")
						}
						return nil, nil
					}
				}
				t.Fatalf("unexpected command %s %v", command, args)
				return nil, nil
			}
			ctx := context.Background()
			if err := r.PrepareDatabase(ctx, target); err == nil {
				t.Fatal("failed database apply accepted")
			}
			if writes != 1 || !strings.Contains(secret, pendingDatabaseHost) {
				t.Fatal("retry credential was not persisted before apply")
			}
			if err := r.Infra(ctx, target, false, false); err == nil {
				t.Fatal("incomplete database accepted for workload deployment")
			}
			failApply = false
			if err := r.PrepareDatabase(ctx, target); err != nil {
				t.Fatal(err)
			}
			if writes != 2 || strings.Contains(secret, pendingDatabaseHost) || applies != 2 {
				t.Fatal("retry did not finish with the same credential")
			}
			if err := r.PrepareDatabase(ctx, target); err != nil {
				t.Fatal(err)
			}
			if writes != 2 {
				t.Fatal("unchanged setup created another secret version")
			}
			if err := r.Infra(ctx, target, false, false); err != nil {
				t.Fatal(err)
			}
			readOnly = true
			if err := r.Infra(ctx, target, false, true); err != nil {
				t.Fatal(err)
			}
			if err := r.Infra(ctx, target, true, true); err == nil {
				t.Fatal("read-only apply accepted")
			}
			readOnly, denyAccess = false, true
			before := secret
			if err := r.PrepareDatabase(ctx, target); err == nil {
				t.Fatal("access failure treated as first setup")
			}
			if secret != before || writes != 2 {
				t.Fatal("access failure rotated credential")
			}
		})
	}
}

func TestDatabaseURLValidationDoesNotLeakCredentials(t *testing.T) {
	password := "NeverPrint:/@ThisPassword"
	valid := databaseURL("10.0.0.1", password)
	_, got, err := parseDatabaseURL(valid)
	if err != nil || got != password {
		t.Fatal("password did not round trip")
	}
	for _, value := range []string{"postgresql://other:" + password + "@host:5432/app", "postgresql://host:5432/app", "postgresql://%zz:" + password + "@host/app", strings.Replace(valid, "sslmode=require", "sslmode=disable", 1)} {
		_, _, err := parseDatabaseURL(value)
		if err == nil || strings.Contains(err.Error(), password) {
			t.Fatal("invalid connection accepted or leaked")
		}
	}
}
