// Package release prepares deployment foundations and deploys immutable images
// using native cloud CLIs.
package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Target is generated from the blueprint; cloud resource identities are resolved
// from the initialized environment's OpenTofu outputs at deployment time.
type Target struct {
	App               string    `json:"app"`
	VCS               string    `json:"vcs,omitempty"`
	Repo              string    `json:"repo,omitempty"`
	Environment       string    `json:"environment"`
	Cloud             string    `json:"cloud"`
	Region            string    `json:"region"`
	Container         string    `json:"container,omitempty"`
	Runtime           string    `json:"runtime"`
	Namespace         string    `json:"namespace"`
	Services          []Service `json:"services"`
	FoundationModules []string  `json:"foundation_modules,omitempty"`
	DatabaseSecret    string    `json:"database_secret,omitempty"`
}
type Service struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	HealthPath string `json:"health_path,omitempty"`
}

// Artifact is user-owned release intent. Promotion copies the digest, never
// rebuilds. Deployment receipts are written only after verification succeeds.
type Artifact struct {
	Version             int    `json:"version"`
	App                 string `json:"app"`
	Environment         string `json:"environment"`
	Image               string `json:"image"`
	SourceImage         string `json:"source_image,omitempty"`
	SourceEnvironment   string `json:"source_environment,omitempty"`
	HealthURL           string `json:"health_url,omitempty"`
	ExpectedVersion     string `json:"expected_version,omitempty"`
	ExpectedEnvironment string `json:"expected_environment,omitempty"`
	RequireDatabase     bool   `json:"require_database,omitempty"`
}

var imageRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]*@sha256:[0-9a-f]{64}$`)

func (a Artifact) Validate(t Target) error {
	if a.Version != 1 || a.App != t.App || a.Environment != t.Environment {
		return fmt.Errorf("release identity does not match this application/environment")
	}
	if !imageRE.MatchString(a.Image) {
		return fmt.Errorf("release image must be a registry/repository@sha256:<64 lowercase hex digits>, never a tag")
	}
	if a.SourceImage != "" && (!imageRE.MatchString(a.SourceImage) || strings.Split(a.SourceImage, "@")[1] != strings.Split(a.Image, "@")[1]) {
		return fmt.Errorf("promotion source must have the same immutable digest")
	}
	if a.SourceImage != "" && (!validEnv(a.SourceEnvironment) || a.SourceEnvironment == a.Environment) {
		return fmt.Errorf("promotion must identify a different source environment")
	}
	if a.SourceEnvironment != "" && a.SourceImage == "" {
		return fmt.Errorf("source_environment requires source_image")
	}
	if a.HealthURL != "" {
		u, err := url.Parse(a.HealthURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("release health_url must be an HTTPS URL without credentials or a fragment")
		}
	}
	if (a.ExpectedVersion != "" || a.ExpectedEnvironment != "" || a.RequireDatabase) && a.HealthURL == "" {
		return fmt.Errorf("version/environment/database verification requires health_url")
	}
	if a.ExpectedEnvironment != "" && a.ExpectedEnvironment != t.Environment {
		return fmt.Errorf("expected_environment must match the deployment target")
	}
	return nil
}
func ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON document in %s", path)
	}
	return nil
}
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".release-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Executor allows offline tests to check cloud calls and failure behavior without
// real credentials; the production implementation never shells command strings.
type Executor func(context.Context, string, []string, []byte, ...string) ([]byte, error)

func Execute(ctx context.Context, name string, args []string, input []byte, environment ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(string(input))
	cmd.Env = append(os.Environ(), "AWS_PAGER=", "CLOUDSDK_CORE_DISABLE_PROMPTS=1")
	cmd.Env = append(cmd.Env, environment...)
	out, err := cmd.Output()
	if err != nil {
		if name == "tofu" {
			var exit *exec.ExitError
			if errors.As(err, &exit) && len(exit.Stderr) != 0 {
				diagnostic := string(exit.Stderr)
				// Infrastructure receives the database password through explicit
				// environment entries. Preserve provider diagnostics while masking
				// those values even if a provider includes one in an error message.
				for _, entry := range environment {
					_, value, ok := strings.Cut(entry, "=")
					if ok && value != "" {
						diagnostic = strings.ReplaceAll(diagnostic, value, "[redacted]")
					}
				}
				return nil, fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(diagnostic))
			}
		}
		return nil, fmt.Errorf("%s %s failed: %w (inspect the cloud operation before retrying)", name, strings.Join(args, " "), err)
	}
	return out, nil
}

type Receipt struct {
	Artifact   Artifact `json:"artifact"`
	VerifiedAt string   `json:"verified_at"`
}

type Runner struct {
	Run  Executor
	Root string
}

func (r Runner) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	return r.Run(ctx, name, args, nil)
}
func (r Runner) outputs(ctx context.Context, env string) (map[string]json.RawMessage, error) {
	out, err := r.command(ctx, "tofu", "-chdir="+filepath.Join(r.Root, "infra/envs", env), "output", "-json", "deployment")
	if err != nil {
		return nil, err
	}
	var values map[string]json.RawMessage
	err = json.Unmarshal(out, &values)
	return values, err
}
func outputString(values map[string]json.RawMessage, key string) (string, error) {
	var value string
	if err := json.Unmarshal(values[key], &value); err != nil || value == "" {
		return "", fmt.Errorf("deployment output %q is missing; complete infrastructure setup first", key)
	}
	return value, nil
}
func (t Target) Validate(env string) error {
	if t.App == "" || t.Environment != env || !validEnv(env) || t.Region == "" {
		return fmt.Errorf("invalid deployment target identity")
	}
	if t.Cloud != "aws" && t.Cloud != "gcp" && t.Cloud != "azure" {
		return fmt.Errorf("unsupported cloud %q", t.Cloud)
	}
	if t.Runtime != "serverless-containers" && t.Runtime != "kubernetes" {
		return fmt.Errorf("unsupported runtime %q", t.Runtime)
	}
	if t.Cloud != "aws" && t.Container == "" {
		return fmt.Errorf("cloud project/subscription is missing")
	}
	if t.DatabaseSecret != "" && !regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`).MatchString(t.DatabaseSecret) {
		return fmt.Errorf("invalid managed database connection secret")
	}
	if len(t.Services) == 0 {
		return fmt.Errorf("deployment target has no services")
	}
	names := map[string]bool{}
	for _, s := range t.Services {
		if s.Name == "" || names[s.Name] || (s.Kind != "http" && s.Kind != "worker" && s.Kind != "cron") {
			return fmt.Errorf("invalid or duplicate deployment service %q", s.Name)
		}
		names[s.Name] = true
	}
	return nil
}
func (r Receipt) Validate(t Target) error {
	if _, err := time.Parse(time.RFC3339, r.VerifiedAt); err != nil {
		return fmt.Errorf("receipt has no valid verification time")
	}
	return r.Artifact.Validate(t)
}
func validEnv(env string) bool { return env == "dev" || env == "stg" || env == "prd" }
func (r Runner) Load(env string) (Target, Artifact, error) {
	var t Target
	var a Artifact
	if !validEnv(env) {
		return t, a, fmt.Errorf("environment must be dev, stg, or prd")
	}
	if err := ReadJSON(filepath.Join(r.Root, ".neckbeard/deploy", env+".json"), &t); err != nil {
		return t, a, err
	}
	if err := ReadJSON(filepath.Join(r.Root, "releases", env+".json"), &a); err != nil {
		return t, a, err
	}
	if err := t.Validate(env); err != nil {
		return t, a, err
	}
	return t, a, a.Validate(t)
}
func (r Runner) Deploy(ctx context.Context, env string) error {
	t, a, err := r.Load(env)
	if err != nil {
		return err
	}
	for _, service := range t.Services {
		if service.Kind == "http" && a.HealthURL == "" {
			return fmt.Errorf("HTTP deployment requires an HTTPS health_url before any runtime changes")
		}
	}
	path := filepath.Join(r.Root, ".neckbeard/deploy", env+".receipt.json")
	var old Receipt
	if err := ReadJSON(path, &old); err == nil {
		if err := old.Validate(t); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if r.Run == nil {
		r.Run = Execute
	}
	lock := filepath.Join(r.Root, ".neckbeard/deploy", env+".lock")
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("deployment is locked: %w; investigate any interrupted run before removing %s", err, lock)
	}
	f.Close()
	defer os.Remove(lock)
	values, err := r.outputs(ctx, env)
	if err != nil {
		return err
	}
	switch {
	case t.Runtime == "kubernetes":
		err = r.kubernetes(ctx, t, a, values)
	case t.Cloud == "aws":
		err = r.aws(ctx, t, a, values)
	case t.Cloud == "gcp":
		err = r.gcp(ctx, t, a, values)
	case t.Cloud == "azure":
		err = r.azure(ctx, t, a, values)
	default:
		err = fmt.Errorf("unsupported release target %s/%s", t.Cloud, t.Runtime)
	}
	if err != nil {
		return err
	}
	if err = waitReady(ctx, 5*time.Minute, func() error { return verifyHealth(ctx, a) }); err != nil {
		return err
	}
	if old.VerifiedAt != "" && old.Artifact.Image != a.Image {
		if err = WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy", env+".previous.json"), old); err != nil {
			return err
		}
	}
	return WriteJSON(path, Receipt{a, time.Now().UTC().Format(time.RFC3339)})
}

func waitReady(ctx context.Context, timeout time.Duration, check func() error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		err := check()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("verification did not converge: %w (%v)", err, ctx.Err())
		case <-time.After(3 * time.Second):
		}
	}
}
