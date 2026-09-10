package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func (r Runner) kubernetes(ctx context.Context, t Target, a Artifact, values map[string]json.RawMessage) error {
	host := map[string]string{"github": "github.com", "gitlab": "gitlab.com"}[t.VCS]
	if host == "" || !regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+$`).MatchString(t.Repo) {
		return fmt.Errorf("Kubernetes release requires a valid source repository")
	}
	status, err := r.command(ctx, "git", "-C", r.Root, "status", "--porcelain", "--untracked-files=normal", "--", "releases", "clusters", ".neckbeard/deploy/"+t.Environment+".json")
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(status))) != 0 {
		return fmt.Errorf("commit the release intent and manifests before deploying through Flux")
	}
	commit, err := r.command(ctx, "git", "-C", r.Root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	revision := strings.TrimSpace(string(commit))
	if !regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`).MatchString(revision) {
		return fmt.Errorf("release has no valid source commit")
	}
	kubeconfig := filepath.Join(r.Root, ".neckbeard/deploy", t.Environment+".kubeconfig")
	cluster, err := outputString(values, "cluster_name")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(kubeconfig), 0700); err != nil {
		return err
	}
	// Use the resolved cluster and an environment-local file, never the user's
	// default kubectl context. Native helpers retain short-lived cloud auth.
	switch t.Cloud {
	case "aws":
		_, err = r.command(ctx, "aws", "eks", "update-kubeconfig", "--name", cluster, "--region", t.Region, "--kubeconfig", kubeconfig)
	case "gcp":
		_, err = r.command(ctx, "env", "KUBECONFIG="+kubeconfig, "gcloud", "container", "clusters", "get-credentials", cluster, "--region", t.Region, "--project", t.Container)
	case "azure":
		group, groupErr := outputString(values, "resource_group")
		if groupErr != nil {
			return groupErr
		}
		_, err = r.command(ctx, "az", "aks", "get-credentials", "--name", cluster, "--resource-group", group, "--subscription", t.Container, "--file", kubeconfig, "--overwrite-existing", "--format", "exec")
		if err == nil {
			_, err = r.command(ctx, "kubelogin", "convert-kubeconfig", "--kubeconfig", kubeconfig, "--login", "azurecli")
		}
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(kubeconfig, 0600); err != nil {
		return err
	}
	// Only the approved environment job can advance this source. Following main
	// directly would let Flux deploy a promotion before the CI approval gate.
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"url": "ssh://git@" + host + "/" + t.Repo, "ref": map[string]string{"commit": revision}}})
	if _, err := r.Run(ctx, "kubectl", []string{"--kubeconfig", kubeconfig, "-n", "flux-system", "patch", "gitrepository", t.App + "-release", "--type=merge", "--patch-file=/dev/stdin"}, patch); err != nil {
		return err
	}
	// Flux remains the sole writer of workloads; CI updates only its source pin.
	if _, err := r.command(ctx, "flux", "reconcile", "kustomization", t.App+"-apps", "--with-source", "--timeout=15m", "--kubeconfig", kubeconfig); err != nil {
		return err
	}
	for _, s := range t.Services {
		kind := "deployment"
		if s.Kind == "cron" {
			kind = "cronjob"
		}
		if kind == "deployment" {
			if _, err := r.command(ctx, "kubectl", "--kubeconfig", kubeconfig, "-n", t.Namespace, "rollout", "status", "deployment/"+s.Name, "--timeout=10m"); err != nil {
				return err
			}
		}
		data, err := r.command(ctx, "kubectl", "--kubeconfig", kubeconfig, "-n", t.Namespace, "get", kind, s.Name, "-o", "json")
		if err != nil {
			return err
		}
		var doc struct {
			Spec any `json:"spec"`
		}
		if err = json.Unmarshal(data, &doc); err != nil {
			return err
		}
		if !hasImage(doc.Spec, a.Image) {
			return fmt.Errorf("%s/%s has not reconciled the requested immutable image; commit and push the release overlay", kind, s.Name)
		}
		if kind == "deployment" {
			data, err = r.command(ctx, "kubectl", "--kubeconfig", kubeconfig, "-n", t.Namespace, "get", "pods", "-l", "app="+s.Name, "-o", "json")
			if err != nil {
				return err
			}
			var pods struct {
				Items []struct {
					Metadata struct {
						DeletionTimestamp string `json:"deletionTimestamp"`
					} `json:"metadata"`
					Status struct {
						ContainerStatuses []struct {
							Name    string `json:"name"`
							Ready   bool   `json:"ready"`
							ImageID string `json:"imageID"`
						} `json:"containerStatuses"`
					} `json:"status"`
				} `json:"items"`
			}
			if err := json.Unmarshal(data, &pods); err != nil {
				return err
			}
			verified := 0
			for _, pod := range pods.Items {
				if pod.Metadata.DeletionTimestamp != "" {
					continue
				}
				found := false
				for _, c := range pod.Status.ContainerStatuses {
					if c.Name == s.Name && c.Ready && strings.HasSuffix(c.ImageID, "@"+strings.Split(a.Image, "@")[1]) {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("pod for %s does not report the ready requested image digest", s.Name)
				}
				verified++
			}
			if verified == 0 {
				return fmt.Errorf("no running pods verify the requested image for %s", s.Name)
			}
		}

	}
	return nil
}
