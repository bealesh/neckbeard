package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ReleaseStore reuses the environment's versioned infrastructure state storage.
// Its location is non-secret; only the cloud deployment identity can write it.
type ReleaseStore struct {
	URL string `json:"url"`
}

type storedReceipts struct {
	Current  Receipt  `json:"current"`
	Previous *Receipt `json:"previous,omitempty"`
}

func (s ReleaseStore) location(t Target) (*url.URL, error) {
	u, err := url.Parse(s.URL)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return nil, fmt.Errorf("invalid release storage location")
	}
	valid := false
	switch t.Cloud {
	case "aws", "gcp":
		scheme := map[string]string{"aws": "s3", "gcp": "gs"}[t.Cloud]
		valid = u.Scheme == scheme && regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).MatchString(u.Host) && (u.Path == "" || u.Path == "/")
	case "azure":
		valid = u.Scheme == "https" && regexp.MustCompile(`^[a-z0-9]+\.blob\.core\.windows\.net$`).MatchString(u.Host) && regexp.MustCompile(`^/[a-z0-9][a-z0-9-]+[a-z0-9]$`).MatchString(u.Path)
	}
	if !valid {
		return nil, fmt.Errorf("release storage location does not match the target cloud")
	}
	return u, nil
}

// SyncReceipts moves a single atomic bundle, so a failed upload cannot publish
// half of a rollback pair. CI serializes this with deployment by environment.
// ponytail: CI resource groups serialize writers; concurrent manual deployments
// require a distributed lock before they can share this storage safely.
func (r Runner) SyncReceipts(ctx context.Context, t Target, upload bool) error {
	if err := t.Validate(t.Environment); err != nil {
		return err
	}
	if r.Run == nil {
		r.Run = Execute
	}
	dir := filepath.Join(r.Root, ".neckbeard/deploy")
	var store ReleaseStore
	if err := ReadJSON(filepath.Join(dir, t.Environment+".store.json"), &store); err != nil {
		return fmt.Errorf("release storage is not configured; run configure-ci and commit its store file: %w", err)
	}
	u, err := store.location(t)
	if err != nil {
		return err
	}
	key := "neckbeard-" + t.Environment + "-receipts.json"
	currentPath := filepath.Join(dir, t.Environment+".receipt.json")
	previousPath := filepath.Join(dir, t.Environment+".previous.json")
	var bundle storedReceipts
	if upload {
		if err := ReadJSON(currentPath, &bundle.Current); err != nil {
			return err
		}
		var previous Receipt
		if err := ReadJSON(previousPath, &previous); err == nil {
			bundle.Previous = &previous
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := bundle.validate(t); err != nil {
			return err
		}
	} else {
		exists, err := r.receiptExists(ctx, t, u, key)
		if err != nil {
			return err // Access errors must never look like the first deployment.
		}
		if !exists {
			for _, path := range []string{currentPath, previousPath} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					return fmt.Errorf("remote receipts are missing but local history exists; investigate before deployment")
				}
			}
			return nil
		}
	}
	work, err := os.MkdirTemp("", "neckbeard-receipts-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	file := filepath.Join(work, "receipts.json")
	if upload {
		if err := WriteJSON(file, bundle); err != nil {
			return err
		}
	}
	if err := r.transferReceipts(ctx, t, u, key, file, upload); err != nil {
		return err
	}
	if upload {
		return nil
	}
	if err := ReadJSON(file, &bundle); err != nil {
		return err
	}
	if err := bundle.validate(t); err != nil {
		return err
	}
	if err := WriteJSON(currentPath, bundle.Current); err != nil {
		return err
	}
	if bundle.Previous != nil {
		return WriteJSON(previousPath, bundle.Previous)
	}
	if err := os.Remove(previousPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (b storedReceipts) validate(t Target) error {
	if err := b.Current.Validate(t); err != nil {
		return err
	}
	if b.Previous != nil {
		return b.Previous.Validate(t)
	}
	return nil
}

func (r Runner) receiptExists(ctx context.Context, t Target, u *url.URL, key string) (bool, error) {
	var out []byte
	var err error
	switch t.Cloud {
	case "aws":
		out, err = r.command(ctx, "aws", "s3api", "list-objects-v2", "--bucket", u.Host, "--prefix", key, "--region", t.Region, "--output", "json")
		if err != nil {
			return false, err
		}
		var listing struct{ Contents []struct{ Key string } }
		if err := json.Unmarshal(out, &listing); err != nil {
			return false, err
		}
		for _, object := range listing.Contents {
			if object.Key == key {
				return true, nil
			}
		}
	case "gcp":
		out, err = r.command(ctx, "gcloud", "storage", "objects", "list", "gs://"+u.Host+"/"+key, "--format=json(name)", "--project", t.Container)
		if err != nil {
			return false, err
		}
		var listing []struct{ Name string }
		if err := json.Unmarshal(out, &listing); err != nil {
			return false, err
		}
		for _, object := range listing {
			if object.Name == key {
				return true, nil
			}
		}
	case "azure":
		args := append([]string{"storage", "blob", "exists"}, azureBlobArgs(t, u, key)...)
		out, err = r.command(ctx, "az", args...)
		if err != nil {
			return false, err
		}
		var result struct{ Exists *bool }
		if err := json.Unmarshal(out, &result); err != nil || result.Exists == nil {
			return false, fmt.Errorf("invalid Azure blob existence response")
		}
		return *result.Exists, nil
	}
	return false, nil
}

func azureBlobArgs(t Target, u *url.URL, key string) []string {
	return []string{"--account-name", strings.TrimSuffix(u.Host, ".blob.core.windows.net"), "--container-name", strings.TrimPrefix(u.Path, "/"), "--name", key, "--auth-mode", "login", "--subscription", t.Container, "--output", "json"}
}

func (r Runner) transferReceipts(ctx context.Context, t Target, u *url.URL, key, file string, upload bool) error {
	remote := strings.TrimRight(u.String(), "/") + "/" + key
	source, dest := remote, file
	if upload {
		source, dest = file, remote
	}
	var err error
	switch t.Cloud {
	case "aws":
		_, err = r.command(ctx, "aws", "s3", "cp", source, dest, "--region", t.Region, "--only-show-errors")
	case "gcp":
		_, err = r.command(ctx, "gcloud", "storage", "cp", source, dest, "--project", t.Container, "--quiet")
	case "azure":
		action := "download"
		if upload {
			action = "upload"
		}
		args := append([]string{"storage", "blob", action}, azureBlobArgs(t, u, key)...)
		args = append(args, "--file", file, "--overwrite", "true", "--no-progress")
		_, err = r.command(ctx, "az", args...)
	}
	return err
}
