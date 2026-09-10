package release

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Main(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: neckbeard release <pin|promote|rollback|stage|deploy|configure-ci|restore-receipts|save-receipts|foundation-plan|foundation-apply|prepare-database|infra-plan|infra-apply> -env <dev|stg|prd>")
	}
	fs := flag.NewFlagSet("release "+args[0], flag.ContinueOnError)
	root := fs.String("root", ".", "application repository")
	env := fs.String("env", "dev", "target environment")
	registry := fs.String("registry", "", "destination registry/repository for promotion; defaults to infrastructure output")
	image := fs.String("image", "", "immutable registry/repository@sha256:digest")
	from := fs.String("from", "", "verified source environment for promotion")
	health := fs.String("health-url", "", "HTTPS endpoint to verify after deployment")
	checkEnvironment := fs.Bool("check-environment", false, "require env to match the target in the health response")
	database := fs.Bool("require-database", false, "require db=ok in the HTTPS health response")
	version := fs.String("expected-version", "", "optional version expected in the health response")
	readOnly := fs.Bool("read-only", false, "infrastructure plan without state locking or database secret access")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if !validEnv(*env) {
		return fmt.Errorf("environment must be dev, stg, or prd")
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	r := Runner{Root: abs, Run: Execute}
	if args[0] == "deploy" || args[0] == "stage" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if args[0] == "stage" {
			return r.Stage(ctx, *env)
		}
		return r.Deploy(ctx, *env)
	}
	var t Target
	if err := ReadJSON(filepath.Join(abs, ".neckbeard/deploy", *env+".json"), &t); err != nil {
		return err
	}
	if err := t.Validate(*env); err != nil {
		return err
	}
	if args[0] == "configure-ci" {
		return r.ConfigureCI(context.Background(), t)
	}
	if args[0] == "prepare-database" || args[0] == "infra-plan" || args[0] == "infra-apply" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancel()
		if *readOnly && args[0] != "infra-plan" {
			return fmt.Errorf("-read-only is only valid for infra-plan")
		}
		if args[0] == "prepare-database" {
			return r.PrepareDatabase(ctx, t)
		}
		return r.Infra(ctx, t, args[0] == "infra-apply", *readOnly)
	}
	if args[0] == "foundation-plan" || args[0] == "foundation-apply" {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		return r.Foundation(ctx, t, args[0] == "foundation-apply")
	}
	if args[0] == "restore-receipts" || args[0] == "save-receipts" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return r.SyncReceipts(ctx, t, args[0] == "save-receipts")
	}
	var a Artifact
	path := filepath.Join(abs, "releases", *env+".json")
	if err := ReadJSON(path, &a); err != nil && !os.IsNotExist(err) {
		return err
	}
	switch args[0] {
	case "pin":
		if *health == "" {
			*health = a.HealthURL
		}
		a = Artifact{Version: 1, App: t.App, Environment: *env, Image: *image, HealthURL: *health, ExpectedVersion: *version, RequireDatabase: *database}
		if *checkEnvironment {
			a.ExpectedEnvironment = *env
		}
	case "promote":
		if !validEnv(*from) || *from == *env {
			return fmt.Errorf("-from must name a different verified environment")
		}
		if !((*from == "dev" && *env == "stg") || (*from == "stg" && *env == "prd")) {
			return fmt.Errorf("promotion order is dev → stg → prd")
		}
		var receipt Receipt
		if err := ReadJSON(filepath.Join(abs, ".neckbeard/deploy", *from+".receipt.json"), &receipt); err != nil {
			return fmt.Errorf("source has no verified deployment receipt: %w", err)
		}
		sourceTarget, _, err := r.Load(*from)
		if err != nil {
			return err
		}
		if sourceTarget.App != t.App {
			return fmt.Errorf("source is a different application")
		}
		if err := receipt.Validate(sourceTarget); err != nil {
			return err
		}
		destination := *registry
		if destination == "" {
			var err error
			destination, err = r.Registry(context.Background(), t)
			if err != nil {
				return err
			}
		}
		a.RequireDatabase = receipt.Artifact.RequireDatabase
		a.Version, a.App, a.Environment, a.Image, a.ExpectedVersion = 1, t.App, *env, receipt.Artifact.Image, receipt.Artifact.ExpectedVersion
		if receipt.Artifact.ExpectedEnvironment != "" {
			a.ExpectedEnvironment = *env
		}
		a.SourceImage = receipt.Artifact.Image
		a.SourceEnvironment = *from
		a.Image = destination + "@" + strings.Split(receipt.Artifact.Image, "@")[1]
		if *health != "" {
			a.HealthURL = *health
		}
	case "rollback":
		var receipt Receipt
		if err := ReadJSON(filepath.Join(abs, ".neckbeard/deploy", *env+".receipt.json"), &receipt); err != nil {
			return fmt.Errorf("no current verified deployment: %w", err)
		}
		if err := receipt.Validate(t); err != nil {
			return err
		}
		// A failed or unexecuted update has not replaced the current receipt.
		// Recover to that last successful release, not the older history entry.
		// After a successful update, current intent and receipt match, so use
		// the previous verified release as the rollback target.
		if a.Image == receipt.Artifact.Image {
			if err := ReadJSON(filepath.Join(abs, ".neckbeard/deploy", *env+".previous.json"), &receipt); err != nil {
				return fmt.Errorf("no previous verified deployment: %w", err)
			}
			if err := receipt.Validate(t); err != nil {
				return err
			}
		}
		a = receipt.Artifact
	default:
		return fmt.Errorf("unknown release action %q", args[0])
	}
	if err := a.Validate(t); err != nil {
		return err
	}
	var overlay map[string]any
	overlayPath := filepath.Join(abs, "releases", *env, "kustomization.json")
	if t.Runtime == "kubernetes" {
		if err := ReadJSON(overlayPath, &overlay); err != nil && !os.IsNotExist(err) {
			return err
		}
		if overlay == nil {
			overlay = map[string]any{"apiVersion": "kustomize.config.k8s.io/v1beta1", "kind": "Kustomization", "resources": []string{"../../clusters/" + *env + "/apps"}}
		}
		parts := strings.Split(a.Image, "@")
		images, ok := overlay["images"].([]any)
		if overlay["images"] != nil && !ok {
			return fmt.Errorf("release overlay images must be an array")
		}
		updated := false
		for i, item := range images {
			entry, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("release overlay image must be an object")
			}
			if entry["name"] == "public.ecr.aws/docker/library/busybox" {
				entry["newName"], entry["digest"] = parts[0], parts[1]
				delete(entry, "newTag")
				images[i], updated = entry, true
			}
		}
		if !updated {
			images = append(images, map[string]string{"name": "public.ecr.aws/docker/library/busybox", "newName": parts[0], "digest": parts[1]})
		}
		overlay["images"] = images
	}
	if err := WriteJSON(path, a); err != nil {
		return err
	}
	// OpenTofu receives the first real image explicitly. Subsequent infra plans
	// retain this input; native deployments own the runtime's image revision.
	if err := WriteJSON(filepath.Join(abs, "releases", *env+".tfvars.json"), map[string]string{"app_image": a.Image}); err != nil {
		return err
	}
	if t.Runtime == "kubernetes" {
		if err := WriteJSON(overlayPath, overlay); err != nil {
			return err
		}
	}
	fmt.Printf("Release intent written for %s: %s. Review and commit releases/ before deployment; production uses the protected CI gate.\n", *env, a.Image)
	return nil
}
