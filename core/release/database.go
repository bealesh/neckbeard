package release

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const pendingDatabaseHost = "pending.invalid"

// PrepareDatabase persists a credential before creating the database so a failed
// apply can reuse it. The temporary connection URL is never deployable. Only a
// successful database apply replaces its host with the actual private endpoint.
// Values travel through stdin or the child's environment, never arguments/files.
func (r Runner) PrepareDatabase(ctx context.Context, t Target) error {
	if err := t.Validate(t.Environment); err != nil {
		return err
	}
	if t.DatabaseSecret == "" {
		return fmt.Errorf("this target has no managed database connection; re-plan and scaffold a provisioned PostgreSQL need first")
	}
	dir := filepath.Join(r.Root, "infra/envs", t.Environment)
	if _, err := os.Stat(filepath.Join(dir, "backend.hcl")); err != nil {
		return fmt.Errorf("complete bootstrap before database setup: %w", err)
	}
	if _, err := r.command(ctx, "tofu", "-chdir="+dir, "init", "-input=false", "-backend-config=backend.hcl"); err != nil {
		return err
	}
	location, err := r.databaseSecretLocation(ctx, t)
	if err != nil {
		return err
	}
	value, exists, err := r.readDatabaseSecret(ctx, t, location)
	if err != nil {
		return err
	}
	if !exists {
		var bytes [32]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return err
		}
		// Fixed character classes satisfy PostgreSQL Flexible Server's password
		// rules; all entropy comes from crypto/rand, not this prefix.
		password := "Nb9_" + base64.RawURLEncoding.EncodeToString(bytes[:])
		value = databaseURL(pendingDatabaseHost, password)
		if err := r.writeDatabaseSecret(ctx, t, location, value); err != nil {
			return err
		}
	}
	u, password, err := parseDatabaseURL(value)
	if err != nil {
		return err
	}
	if u.Hostname() != pendingDatabaseHost {
		if err := r.verifyDatabaseHost(ctx, t, u); err != nil {
			return err
		}
	}
	planPath := filepath.Join(dir, "database.tfplan")
	if err := os.Remove(planPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	defer os.Remove(planPath)
	tofu := func(args ...string) ([]byte, error) {
		return r.Run(ctx, "tofu", append([]string{"-chdir=" + dir}, args...), nil, "TF_VAR_database_password="+password)
	}
	out, err := tofu("plan", "-input=false", "-target=module.postgres", "-out=database.tfplan")
	if err != nil {
		return err
	}
	fmt.Print(string(out))
	plan, err := tofu("show", "-json", "database.tfplan")
	if err != nil {
		return err
	}
	var dependencies []string
	if t.Cloud == "aws" && t.Runtime == "serverless-containers" {
		dependencies = []string{"module.runtime_serverless.aws_security_group.service"}
	}
	if err := validateFoundationPlan(plan, t.Cloud, map[string]bool{"postgres": true, "network": true}, dependencies...); err != nil {
		return fmt.Errorf("database setup only creates resources; use the full infrastructure workflow for existing database changes: %w", err)
	}
	if _, err := tofu("apply", "-input=false", "database.tfplan"); err != nil {
		return err
	}
	host, err := r.databaseHost(ctx, t)
	if err != nil {
		return err
	}
	complete := databaseURL(host, password)
	if value != complete {
		if err := r.writeDatabaseSecret(ctx, t, location, complete); err != nil {
			return err
		}
	}
	fmt.Printf("Database connection prepared in %s for %s; no credential was written locally.\n", t.DatabaseSecret, t.Environment)
	return nil
}

func databaseURL(host, password string) string {
	u := url.URL{Scheme: "postgresql", User: url.UserPassword("neckbeard", password), Host: net.JoinHostPort(host, "5432"), Path: "/app", RawQuery: "sslmode=require"}
	return u.String()
}

func parseDatabaseURL(value string) (*url.URL, string, error) {
	u, err := url.Parse(value)
	if err != nil {
		return nil, "", fmt.Errorf("database connection secret is not a valid URL")
	}
	password, hasPassword := u.User.Password()
	if (u.Scheme != "postgresql" && u.Scheme != "postgres") || u.User.Username() != "neckbeard" || !hasPassword || password == "" || u.Hostname() == "" || u.Port() != "5432" || u.Path != "/app" || u.Fragment != "" || u.Query().Get("sslmode") != "require" {
		return nil, "", fmt.Errorf("existing connection secret is not a neckbeard-managed PostgreSQL connection; migrate it explicitly instead of overwriting it")
	}
	return u, password, nil
}

func (r Runner) databaseHost(ctx context.Context, t Target) (string, error) {
	out, err := r.command(ctx, "tofu", "-chdir="+filepath.Join(r.Root, "infra/envs", t.Environment), "output", "-json", "database")
	if err != nil {
		return "", err
	}
	var db struct{ Host, User, Name string }
	if err := json.Unmarshal(out, &db); err != nil || db.User != "neckbeard" || db.Name != "app" || !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*$`).MatchString(db.Host) {
		return "", fmt.Errorf("missing or invalid database output; complete database setup first")
	}
	return db.Host, nil
}

func (r Runner) verifyDatabaseHost(ctx context.Context, t Target, u *url.URL) error {
	host, err := r.databaseHost(ctx, t)
	if err != nil {
		return err
	}
	if u.Hostname() != host {
		return fmt.Errorf("connection secret points to a different database; refusing to change it")
	}
	return nil
}

func (r Runner) databasePassword(ctx context.Context, t Target) (string, error) {
	location, err := r.databaseSecretLocation(ctx, t)
	if err != nil {
		return "", err
	}
	value, exists, err := r.readDatabaseSecret(ctx, t, location)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("database connection is missing; run prepare-database first")
	}
	u, password, err := parseDatabaseURL(value)
	if err != nil {
		return "", err
	}
	if u.Hostname() == pendingDatabaseHost {
		return "", fmt.Errorf("database setup is incomplete; rerun prepare-database before deploying workloads")
	}
	if err := r.verifyDatabaseHost(ctx, t, u); err != nil {
		return "", err
	}
	return password, nil
}

func (r Runner) databaseSecretLocation(ctx context.Context, t Target) (string, error) {
	out, err := r.command(ctx, "tofu", "-chdir="+filepath.Join(r.Root, "infra/envs", t.Environment), "output", "-json", "secret_locations")
	if err != nil {
		return "", err
	}
	var locations map[string]string
	if err := json.Unmarshal(out, &locations); err != nil {
		return "", fmt.Errorf("invalid secret locations output")
	}
	location := locations[t.DatabaseSecret]
	if t.Cloud == "aws" {
		parts := strings.SplitN(location, ":", 7)
		if len(parts) == 7 && parts[0] == "arn" && parts[1] == "aws" && parts[2] == "secretsmanager" && parts[3] == t.Region && regexp.MustCompile(`^[0-9]{12}$`).MatchString(parts[4]) && parts[5] == "secret" && parts[6] != "" && (t.Container == "" || parts[4] == t.Container) {
			return location, nil
		}
	}
	if t.Cloud == "gcp" && regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_-]*$`).MatchString(location) {
		return location, nil
	}
	if t.Cloud == "azure" {
		if _, _, err := azureSecretParts(location); err == nil {
			return location, nil
		}
	}
	return "", fmt.Errorf("database secret location is missing or invalid; complete foundations first")
}

func azureSecretParts(location string) (string, string, error) {
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" || !regexp.MustCompile(`^[a-zA-Z0-9-]+\.vault\.azure\.net$`).MatchString(u.Host) || !regexp.MustCompile(`^/secrets/[a-zA-Z0-9-]+$`).MatchString(u.Path) {
		return "", "", fmt.Errorf("invalid Key Vault secret URI")
	}
	return strings.TrimSuffix(u.Host, ".vault.azure.net"), strings.TrimPrefix(u.Path, "/secrets/"), nil
}

func (r Runner) readDatabaseSecret(ctx context.Context, t Target, location string) (string, bool, error) {
	if t.Cloud == "aws" {
		out, err := r.command(ctx, "aws", "secretsmanager", "describe-secret", "--secret-id", location, "--region", t.Region, "--output", "json")
		if err != nil {
			return "", false, err
		}
		var secret struct {
			ARN                string
			VersionIdsToStages map[string][]string
		}
		if err := json.Unmarshal(out, &secret); err != nil || secret.ARN != location {
			return "", false, fmt.Errorf("invalid Secrets Manager metadata")
		}
		if len(secret.VersionIdsToStages) == 0 {
			return "", false, nil
		}
		out, err = r.command(ctx, "aws", "secretsmanager", "get-secret-value", "--secret-id", location, "--version-stage", "AWSCURRENT", "--region", t.Region, "--output", "json")
		if err != nil {
			return "", true, err
		}
		var value struct{ SecretString *string }
		if err := json.Unmarshal(out, &value); err != nil || value.SecretString == nil {
			return "", true, fmt.Errorf("invalid Secrets Manager value response")
		}
		return *value.SecretString, true, nil
	}
	if t.Cloud == "gcp" {
		out, err := r.command(ctx, "gcloud", "secrets", "versions", "list", location, "--project", t.Container, "--format=json(name)")
		if err != nil {
			return "", false, err
		}
		var versions []struct{ Name string }
		if err := json.Unmarshal(out, &versions); err != nil || versions == nil {
			return "", false, fmt.Errorf("invalid secret version listing")
		}
		if len(versions) == 0 {
			return "", false, nil
		}
		// Existing disabled/destroyed versions must fail access, never regenerate.
		out, err = r.command(ctx, "gcloud", "secrets", "versions", "access", "latest", "--secret", location, "--project", t.Container)
		return strings.TrimSpace(string(out)), true, err
	}
	vault, name, err := azureSecretParts(location)
	if err != nil {
		return "", false, err
	}
	out, err := r.command(ctx, "az", "keyvault", "secret", "list", "--vault-name", vault, "--subscription", t.Container, "--output", "json")
	if err != nil {
		return "", false, err
	}
	var secrets []struct{ Name string }
	if err := json.Unmarshal(out, &secrets); err != nil || secrets == nil {
		return "", false, fmt.Errorf("invalid Key Vault secret listing")
	}
	for _, secret := range secrets {
		if secret.Name != name {
			continue
		}
		out, err := r.command(ctx, "az", "keyvault", "secret", "show", "--id", location, "--subscription", t.Container, "--output", "json")
		if err != nil {
			return "", true, err
		}
		var value struct{ Value *string }
		if err := json.Unmarshal(out, &value); err != nil || value.Value == nil {
			return "", true, fmt.Errorf("invalid Key Vault secret response")
		}
		return *value.Value, true, nil
	}
	return "", false, nil
}

func (r Runner) writeDatabaseSecret(ctx context.Context, t Target, location, value string) error {
	if t.Cloud == "aws" {
		// --cli-input-json opens its file twice, so a pipe is consumed before
		// execution. The individual string parameter reads stdin only once.
		_, err := r.Run(ctx, "aws", []string{"secretsmanager", "put-secret-value", "--secret-id", location, "--secret-string", "file:///dev/stdin", "--region", t.Region, "--output", "json"}, []byte(value))
		return err
	}
	if t.Cloud == "gcp" {
		_, err := r.Run(ctx, "gcloud", []string{"secrets", "versions", "add", location, "--project", t.Container, "--data-file=-", "--quiet"}, []byte(value))
		return err
	}
	vault, name, err := azureSecretParts(location)
	if err != nil {
		return err
	}
	_, err = r.Run(ctx, "az", []string{"keyvault", "secret", "set", "--vault-name", vault, "--name", name, "--subscription", t.Container, "--file", "/dev/stdin", "--encoding", "utf-8", "--output", "none"}, []byte(value))
	return err
}
