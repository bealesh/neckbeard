package release

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDurableReceiptsAcrossFreshJobs(t *testing.T) {
	for cloud, location := range map[string]string{"aws": "s3://test-bucket", "gcp": "gs://test-bucket", "azure": "https://teststore.blob.core.windows.net/tfstate"} {
		t.Run(cloud, func(t *testing.T) {
			r, target, a := fixture(t, cloud, "worker")
			dir := filepath.Join(r.Root, ".neckbeard/deploy")
			WriteJSON(filepath.Join(dir, "dev.store.json"), ReleaseStore{URL: location})
			currentPath, previousPath := filepath.Join(dir, "dev.receipt.json"), filepath.Join(dir, "dev.previous.json")
			var remote []byte
			failUpload, failListing := false, false
			r.Run = func(_ context.Context, name string, args []string, _ []byte, _ ...string) ([]byte, error) {
				listing := slices.Contains(args, "list-objects-v2") || slices.Contains(args, "list") || slices.Contains(args, "exists")
				if listing {
					if failListing {
						return nil, errors.New("access denied")
					}
					switch name {
					case "aws":
						if remote == nil {
							return []byte(`{"Contents":[]}`), nil
						}
						return []byte(`{"Contents":[{"Key":"neckbeard-dev-receipts.json"}]}`), nil
					case "gcloud":
						if remote == nil {
							return []byte(`[]`), nil
						}
						return []byte(`[{"name":"neckbeard-dev-receipts.json"}]`), nil
					case "az":
						return json.Marshal(map[string]bool{"exists": remote != nil})
					}
				}
				var file string
				upload := false
				if name == "az" {
					i := slices.Index(args, "--file")
					if i < 0 || !slices.Contains(args, "login") {
						t.Fatal("Azure receipt transfer lacks file or federated auth", args)
					}
					file, upload = args[i+1], args[2] == "upload"
				} else {
					upload = !strings.Contains(args[2], "://")
					file = args[3]
					if upload {
						file = args[2]
					}
				}
				if upload {
					if failUpload {
						return nil, errors.New("upload interrupted")
					}
					var err error
					remote, err = os.ReadFile(file)
					return nil, err
				}
				return nil, os.WriteFile(file, remote, 0600)
			}
			ctx := context.Background()
			if err := r.SyncReceipts(ctx, target, false); err != nil {
				t.Fatal("first deployment should have no receipt:", err)
			}
			failListing = true
			if err := r.SyncReceipts(ctx, target, false); err == nil {
				t.Fatal("access error was mistaken for first deployment")
			}
			failListing = false
			old := Receipt{Artifact: a, VerifiedAt: time.Now().UTC().Format(time.RFC3339)}
			WriteJSON(currentPath, old)
			if err := r.SyncReceipts(ctx, target, true); err != nil {
				t.Fatal(err)
			}
			next := old
			next.Artifact.Image = nextImage
			WriteJSON(currentPath, next)
			WriteJSON(previousPath, old)
			failUpload = true
			if err := r.SyncReceipts(ctx, target, true); err == nil {
				t.Fatal("failed receipt upload accepted")
			}
			os.Remove(currentPath)
			os.Remove(previousPath)
			if err := r.SyncReceipts(ctx, target, false); err != nil {
				t.Fatal(err)
			}
			var restored Receipt
			ReadJSON(currentPath, &restored)
			if restored != old {
				t.Fatal("failed upload replaced known-good remote history")
			}
			failUpload = false
			WriteJSON(currentPath, next)
			WriteJSON(previousPath, old)
			if err := r.SyncReceipts(ctx, target, true); err != nil {
				t.Fatal(err)
			}
			os.Remove(currentPath)
			os.Remove(previousPath)
			if err := r.SyncReceipts(ctx, target, false); err != nil {
				t.Fatal(err)
			}
			ReadJSON(currentPath, &restored)
			if restored != next {
				t.Fatal("fresh job did not restore the current deployment")
			}
			ReadJSON(previousPath, &restored)
			if restored != old {
				t.Fatal("fresh job did not restore rollback history")
			}
			remote = nil
			if err := r.SyncReceipts(ctx, target, false); err == nil {
				t.Fatal("missing remote history accepted over existing local receipts")
			}
			wrong := next
			wrong.Artifact.Environment = "prd"
			remote, _ = json.Marshal(storedReceipts{Current: wrong})
			if err := r.SyncReceipts(ctx, target, false); err == nil {
				t.Fatal("wrong-environment receipt accepted")
			}
			ReadJSON(currentPath, &restored)
			if restored != next {
				t.Fatal("invalid remote receipt changed local history")
			}
		})
	}
}
