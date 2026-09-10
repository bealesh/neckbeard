package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestKubernetesUsesTargetCredentialsAndRunningDigest(t *testing.T) {
	for _, cloud := range []string{"aws", "gcp", "azure"} {
		t.Run(cloud, func(t *testing.T) {
			r, target, a := fixture(t, cloud, "worker")
			target.Runtime = "kubernetes"
			target.VCS, target.Repo = "github", "owner/test"
			target.Namespace = "test-app"
			config := filepath.Join(r.Root, ".neckbeard/deploy/dev.kubeconfig")
			values := map[string]json.RawMessage{"cluster_name": json.RawMessage(`"test-cluster"`), "resource_group": json.RawMessage(`"test-group"`)}
			badDigest := false
			pinned := false
			r.Run = func(_ context.Context, command string, args []string, input []byte, _ ...string) ([]byte, error) {
				if command == "git" {
					if slices.Contains(args, "rev-parse") {
						return []byte(strings.Repeat("a", 40)), nil
					}
					return nil, nil
				}
				if command == "aws" || command == "az" || command == "env" {
					if !slices.Contains(args, "test-cluster") {
						t.Fatal("cluster was not resolved from this environment", args)
					}
					if !slices.Contains(args, config) && !slices.Contains(args, "KUBECONFIG="+config) {
						t.Fatal("export could change the user's default kubeconfig", args)
					}
					if cloud != "aws" && !slices.Contains(args, target.Container) {
						t.Fatal("cloud container was not explicit", args)
					}
					return nil, os.WriteFile(config, []byte("test config"), 0600)
				}
				if !slices.Contains(args, config) {
					t.Fatal("cluster command omitted the environment kubeconfig", command, args)
				}
				if slices.Contains(args, "patch") {
					if !strings.Contains(string(input), strings.Repeat("a", 40)) || !slices.Contains(args, "test-release") {
						t.Fatal("Flux source did not receive the approved checkout commit", args, string(input))
					}
					pinned = true
					return nil, nil
				}
				if command == "flux" && !pinned {
					t.Fatal("workloads reconciled before their approved source was pinned")
				}
				if command == "flux" || command == "kubelogin" || slices.Contains(args, "rollout") {
					return nil, nil
				}
				if slices.Contains(args, "pods") {
					digest := strings.Split(a.Image, "@")[1]
					if badDigest {
						digest = "sha256:" + strings.Repeat("f", 64)
					}
					return []byte(fmt.Sprintf(`{"items":[{"status":{"containerStatuses":[{"name":"web","ready":true,"imageID":"registry.example/app@%s"}]}}]}`, digest)), nil
				}
				return []byte(fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"web","image":%q}]}}}}`, a.Image)), nil
			}
			if err := r.kubernetes(context.Background(), target, a, values); err != nil {
				t.Fatal(err)
			}
			badDigest = true
			if err := r.kubernetes(context.Background(), target, a, values); err == nil {
				t.Fatal("desired template accepted despite a different running image")
			}
		})
	}
}
