package release

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Stage copies an already-built image into the destination registry. Skopeo
// gets short-lived registry credentials from the current cloud identity. The
// private auth file lives only for this operation, never in CI artifacts.
func (r Runner) Stage(ctx context.Context, env string) error {
	t, a, err := r.Load(env)
	if err != nil {
		return err
	}
	if r.Run == nil {
		r.Run = Execute
	}
	authDir, err := os.MkdirTemp("", "neckbeard-registry-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(authDir)
	authFile := filepath.Join(authDir, "auth.json")
	if err := r.registryLogin(ctx, t, a.Image, authFile); err != nil {
		return err
	}
	// Retries and rollbacks need only the already-staged digest. In particular,
	// they must still work after the source registry has been cleaned up.
	check := func() error {
		raw, err := r.command(ctx, "skopeo", "inspect", "--authfile", authFile, "--raw", "docker://"+a.Image)
		if err != nil {
			return err
		}
		if fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) != strings.Split(a.Image, "@")[1] {
			return fmt.Errorf("destination registry did not preserve the release digest")
		}
		return nil
	}
	if err := check(); err == nil || a.SourceImage == "" || a.SourceImage == a.Image {
		return err
	}
	if a.SourceImage != "" && a.SourceImage != a.Image {
		var source Target
		if err := ReadJSON(filepath.Join(r.Root, ".neckbeard/deploy", a.SourceEnvironment+".json"), &source); err != nil {
			return err
		}
		if err := source.Validate(a.SourceEnvironment); err != nil {
			return err
		}
		if source.Cloud != t.Cloud || source.App != t.App {
			return fmt.Errorf("promotion source must belong to the same application and cloud")
		}
		if err := r.registryLogin(ctx, source, a.SourceImage, authFile); err != nil {
			return fmt.Errorf("source registry access: %w", err)
		}
		_, copyErr := r.command(ctx, "skopeo", "copy", "--all", "--preserve-digests", "--authfile", authFile, "docker://"+a.SourceImage, "docker://"+strings.Split(a.Image, "@")[0]+":release-"+strings.TrimPrefix(strings.Split(a.Image, "@")[1], "sha256:"))
		if verifyErr := check(); verifyErr != nil {
			return errors.Join(copyErr, verifyErr)
		}
	}
	return nil
}

func (r Runner) registryLogin(ctx context.Context, t Target, image, authFile string) error {
	host, _, ok := strings.Cut(image, "/")
	if !ok {
		return fmt.Errorf("image must include a cloud registry hostname")
	}
	var token []byte
	var err error
	user := ""
	switch t.Cloud {
	case "aws":
		parts := regexp.MustCompile(`^([0-9]{12})\.dkr\.ecr\.([a-z0-9-]+)\.amazonaws\.com$`).FindStringSubmatch(host)
		if parts == nil || parts[2] != t.Region || (t.Container != "" && parts[1] != t.Container) {
			return fmt.Errorf("ECR image does not belong to the target account and region")
		}
		user = "AWS"
		token, err = r.command(ctx, "aws", "ecr", "get-login-password", "--region", t.Region)
	case "gcp":
		if host != t.Region+"-docker.pkg.dev" || !strings.HasPrefix(image, host+"/"+t.Container+"/") {
			return fmt.Errorf("Artifact Registry image does not belong to the target project and region")
		}
		user = "oauth2accesstoken"
		token, err = r.command(ctx, "gcloud", "auth", "print-access-token", "--quiet")
	case "azure":
		if !regexp.MustCompile(`^[a-z0-9]+\.azurecr\.io$`).MatchString(host) {
			return fmt.Errorf("image is not an Azure Container Registry image")
		}
		var data []byte
		data, err = r.command(ctx, "az", "acr", "login", "--name", strings.TrimSuffix(host, ".azurecr.io"), "--subscription", t.Container, "--expose-token", "--output", "json")
		if err == nil {
			var credentials struct {
				AccessToken string `json:"accessToken"`
				LoginServer string `json:"loginServer"`
			}
			if err := json.Unmarshal(data, &credentials); err != nil {
				return fmt.Errorf("invalid ACR login response")
			}
			if credentials.LoginServer != host {
				return fmt.Errorf("ACR login returned a different registry")
			}
			token = []byte(credentials.AccessToken)
		}
		user = "00000000-0000-0000-0000-000000000000"
	default:
		return fmt.Errorf("unsupported registry cloud %q", t.Cloud)
	}
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(token))) == 0 {
		return fmt.Errorf("cloud returned an empty registry credential")
	}
	_, err = r.Run(ctx, "skopeo", []string{"login", "--authfile", authFile, "--username", user, "--password-stdin", host}, token)
	return err
}

func (r Runner) Registry(ctx context.Context, t Target) (string, error) {
	data, err := r.command(ctx, "tofu", "-chdir="+r.Root+"/infra/envs/"+t.Environment, "output", "-raw", "registry_url")
	if err != nil {
		return "", err
	}
	registry := strings.TrimSpace(string(data))
	if t.Cloud != "aws" {
		registry += "/" + t.App
	}
	if !imageRE.MatchString(registry + "@sha256:" + strings.Repeat("0", 64)) {
		return "", fmt.Errorf("invalid destination registry output")
	}
	return registry, nil
}
