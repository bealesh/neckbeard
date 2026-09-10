package release

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testImage = "registry.example/app@sha256:" + strings.Repeat("a", 64)
var nextImage = "registry.example/app@sha256:" + strings.Repeat("b", 64)

func TestInfrastructureErrorRetainsDiagnosticWithoutPassword(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("requires a POSIX shell")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'permission denied while using %s\\n' \"$TF_VAR_database_password\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "tofu"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	password := "test-password-must-not-appear"
	_, err := Execute(context.Background(), "tofu", []string{"apply"}, nil, "TF_VAR_database_password="+password)
	if err == nil || !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "[redacted]") || strings.Contains(err.Error(), password) {
		t.Fatalf("expected a useful redacted infrastructure diagnostic: %v", err)
	}
}

func fixture(t *testing.T, cloud, kind string) (Runner, Target, Artifact) {
	t.Helper()
	r := Runner{Root: t.TempDir()}
	target := Target{App: "test", Environment: "dev", Cloud: cloud, Runtime: "serverless-containers", Region: "region", Container: "project", Services: []Service{{Name: "web", Kind: kind}}}
	artifact := Artifact{Version: 1, App: "test", Environment: "dev", Image: testImage}
	save := func(p string, v any) {
		t.Helper()
		if err := WriteJSON(filepath.Join(r.Root, p), v); err != nil {
			t.Fatal(err)
		}
	}
	save(".neckbeard/deploy/dev.json", target)
	save("releases/dev.json", artifact)
	return r, target, artifact
}

func TestRejectBeforeCloudMutation(t *testing.T) {
	for _, mode := range []string{"http-without-health", "wrong-target", "empty-services", "malformed-receipt", "locked"} {
		t.Run(mode, func(t *testing.T) {
			r, target, a := fixture(t, "aws", "worker")
			switch mode {
			case "http-without-health":
				target.Services[0].Kind = "http"
			case "wrong-target":
				target.Environment = "prd"
			case "empty-services":
				target.Services = nil
			case "malformed-receipt":
				WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.receipt.json"), Receipt{Artifact: a})
			case "locked":
				os.WriteFile(filepath.Join(r.Root, ".neckbeard/deploy/dev.lock"), nil, 0600)
			}
			WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.json"), target)
			r.Run = func(context.Context, string, []string, []byte, ...string) ([]byte, error) {
				t.Fatal("cloud command before validation")
				return nil, nil
			}
			if err := r.Deploy(context.Background(), "dev"); err == nil {
				t.Fatal("accepted invalid deployment")
			}
		})
	}
}

func TestArtifactAndJSONValidation(t *testing.T) {
	_, target, a := fixture(t, "aws", "worker")
	for _, image := range []string{"repo:latest", "-bad@sha256:" + strings.Repeat("a", 64), testImage + "junk"} {
		a.Image = image
		if a.Validate(target) == nil {
			t.Errorf("accepted %s", image)
		}
	}
	a.Image = testImage
	a.ExpectedEnvironment = "dev"
	if a.Validate(target) == nil {
		t.Fatal("environment check accepted without a health URL")
	}
	a.HealthURL, a.ExpectedEnvironment = "https://test/healthz", "prd"
	if a.Validate(target) == nil {
		t.Fatal("check accepted for a different environment")
	}
	a.ExpectedEnvironment = ""
	for _, u := range []string{"http://test/healthz", "https://", "https://user:secret@test/healthz"} {
		a.HealthURL = u
		if a.Validate(target) == nil {
			t.Errorf("accepted URL %s", u)
		}
	}
	p := filepath.Join(t.TempDir(), "doc.json")
	for _, raw := range []string{`{} {}`, `{"unknown":1}`} {
		os.WriteFile(p, []byte(raw), 0600)
		if ReadJSON(p, &Artifact{}) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestPinPreservesKubernetesCustomizations(t *testing.T) {
	r, target, _ := fixture(t, "aws", "worker")
	target.Runtime = "kubernetes"
	WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.json"), target)
	path := filepath.Join(r.Root, "releases/dev/kustomization.json")
	custom := `{"apiVersion":"kustomize.config.k8s.io/v1beta1","kind":"Kustomization","resources":["../../clusters/dev/apps","tls.yaml"],"patches":[{"path":"environment.yaml"}],"images":[{"name":"sidecar","newTag":"v1"},{"name":"public.ecr.aws/docker/library/busybox","newTag":"obsolete"}]}`
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(custom), 0600)
	for _, image := range []string{testImage, nextImage} {
		if err := Main([]string{"pin", "-root", r.Root, "-image", image}); err != nil {
			t.Fatal(err)
		}
	}
	var overlay map[string]any
	if err := ReadJSON(path, &overlay); err != nil {
		t.Fatal(err)
	}
	if len(overlay["resources"].([]any)) != 2 || len(overlay["patches"].([]any)) != 1 {
		t.Fatal("pin erased TLS or environment customization")
	}
	images := overlay["images"].([]any)
	app := images[1].(map[string]any)
	if len(images) != 2 || images[0].(map[string]any)["newTag"] != "v1" || app["digest"] != strings.Split(nextImage, "@")[1] || app["newTag"] != nil {
		t.Fatal("pin lost sidecar or failed to update the app digest", images)
	}
	before, _ := os.ReadFile(filepath.Join(r.Root, "releases/dev.json"))
	os.WriteFile(path, []byte(`{"images":"invalid"}`), 0600)
	if err := Main([]string{"pin", "-root", r.Root, "-image", testImage}); err == nil {
		t.Fatal("invalid overlay accepted")
	}
	after, _ := os.ReadFile(filepath.Join(r.Root, "releases/dev.json"))
	if string(before) != string(after) {
		t.Fatal("invalid overlay left partially updated release intent")
	}
}

func TestFailedReleasePreservesReceipt(t *testing.T) {
	r, _, a := fixture(t, "azure", "worker")
	path := filepath.Join(r.Root, ".neckbeard/deploy/dev.receipt.json")
	old := Receipt{Artifact: a, VerifiedAt: time.Now().UTC().Format(time.RFC3339)}
	WriteJSON(path, old)
	a.Image = nextImage
	WriteJSON(filepath.Join(r.Root, "releases/dev.json"), a)
	r.Run = func(_ context.Context, name string, args []string, _ []byte, _ ...string) ([]byte, error) {
		if name == "tofu" {
			return []byte(`{"resource_group":"test","service_names":{"web":"test-web"}}`), nil
		}
		return nil, errors.New("rollout failed")
	}
	if err := r.Deploy(context.Background(), "dev"); err == nil {
		t.Fatal("failed deployment accepted")
	}
	var actual Receipt
	if err := ReadJSON(path, &actual); err != nil {
		t.Fatal(err)
	}
	if actual != old {
		t.Fatal("failed deployment replaced known-good receipt")
	}
	if _, err := os.Stat(filepath.Join(r.Root, ".neckbeard/deploy/dev.previous.json")); !os.IsNotExist(err) {
		t.Fatal("failed release created rollback record")
	}
}

func TestAzureReleaseAndRollback(t *testing.T) {
	r, _, a := fixture(t, "azure", "worker")
	current := testImage
	r.Run = func(_ context.Context, name string, args []string, _ []byte, _ ...string) ([]byte, error) {
		if name == "tofu" {
			return []byte(`{"resource_group":"test","service_names":{"web":"test-web"}}`), nil
		}
		if !strings.Contains(strings.Join(args, " "), "--subscription project") {
			t.Fatal("subscription omitted")
		}
		return []byte(fmt.Sprintf(`{"properties":{"provisioningState":"Succeeded","latestRevisionName":"rev","latestReadyRevisionName":"rev","template":{"containers":[{"image":%q}]}}}`, current)), nil
	}
	if err := r.Deploy(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
	current = nextImage
	a.Image = nextImage
	WriteJSON(filepath.Join(r.Root, "releases/dev.json"), a)
	if err := r.Deploy(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
	if err := Main([]string{"rollback", "-root", r.Root, "-env", "dev"}); err != nil {
		t.Fatal(err)
	}
	_, rolled, err := r.Load("dev")
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Image != testImage {
		t.Fatal("rollback did not restore verified digest")
	}
	var previous Receipt
	ReadJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.previous.json"), &previous)
	if previous.Artifact.Image != testImage {
		t.Fatal("wrong previous release")
	}
}

func TestHealthIdentityAndDatabase(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{"ok", `{"version":"v2","db":"ok"}`, 200, false},
		{"old-image", `{"version":"v1","db":"ok"}`, 200, true},
		{"memory-fallback", `{"version":"v2","db":"not-configured"}`, 200, true},
		{"missing-db", `{"version":"v2"}`, 200, true},
		{"bad-status", `{"version":"v2","db":"ok"}`, 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			old := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			defer func() { http.DefaultTransport = old }()
			err := verifyHealth(context.Background(), Artifact{HealthURL: server.URL, ExpectedVersion: "v2", RequireDatabase: true})
			if (err != nil) != tc.wantError {
				t.Fatalf("health error = %v", err)
			}
		})
	}
}

func TestCloudRunObservedReadiness(t *testing.T) {
	base := fmt.Sprintf(`{"metadata":{"generation":2},"spec":{"template":{"spec":{"containers":[{"image":%q}]}}},"status":{"observedGeneration":2,"conditions":[{"type":"Ready","status":"True"}],"latestCreatedRevisionName":"rev2","latestReadyRevisionName":"rev2","traffic":[{"revisionName":"rev2","percent":100}]}}`, testImage)
	for _, tc := range []struct {
		name, old, new string
		bad            bool
	}{
		{"ready", "", "", false},
		{"stale-generation", `"observedGeneration":2`, `"observedGeneration":1`, true},
		{"not-ready", `"status":"True"`, `"status":"False"`, true},
		{"old-revision", `"latestReadyRevisionName":"rev2"`, `"latestReadyRevisionName":"rev1"`, true},
		{"old-traffic", `"revisionName":"rev2"`, `"revisionName":"rev1"`, true},
		{"partial-traffic", `"percent":100`, `"percent":50`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := base
			if tc.old != "" {
				raw = strings.Replace(raw, tc.old, tc.new, 1)
			}
			var doc map[string]any
			json.Unmarshal([]byte(raw), &doc)
			err := cloudRunReady(doc, "http", testImage)
			if (err != nil) != tc.bad {
				t.Fatalf("readiness error = %v", err)
			}
		})
	}
}

func TestAWSRejectsEmptyTaskEvidence(t *testing.T) {
	r, target, a := fixture(t, "aws", "worker")
	values := map[string]json.RawMessage{"cluster_name": json.RawMessage(`"cluster"`), "task_families": json.RawMessage(`{"web":"family"}`)}
	var inputPath string
	r.Run = func(_ context.Context, _ string, args []string, input []byte, _ ...string) ([]byte, error) {
		switch args[1] {
		case "describe-task-definition":
			return []byte(`{"taskDefinition":{"family":"family","containerDefinitions":[{"name":"web","image":"old","environment":[{"name":"KEEP","value":"config"}]}]}}`), nil
		case "register-task-definition":
			inputPath = strings.TrimPrefix(args[3], "file://")
			info, err := os.Stat(inputPath)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("AWS configuration is not a private file")
			}
			input, err = os.ReadFile(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(input), testImage) || !strings.Contains(string(input), "KEEP") {
				t.Fatal("release dropped workload config")
			}
			return []byte(`{"taskDefinition":{"taskDefinitionArn":"arn:new"}}`), nil
		case "update-service", "wait":
			return []byte(`{}`), nil
		case "describe-services":
			return []byte(`{"services":[{"taskDefinition":"arn:new","runningCount":1,"desiredCount":1}]}`), nil
		case "list-tasks":
			return []byte(`{"taskArns":["task"]}`), nil
		case "describe-tasks":
			return []byte(`{"tasks":[],"failures":[{"arn":"task","reason":"MISSING"}]}`), nil
		default:
			t.Fatalf("unexpected command %v", args)
			return nil, nil
		}
	}
	if err := r.aws(context.Background(), target, a, values); err == nil {
		t.Fatal("missing tasks accepted as verified")
	}
	if _, err := os.Stat(inputPath); !os.IsNotExist(err) {
		t.Fatal("AWS task configuration was retained after deployment")
	}
}

func TestStagePreservesDigestAcrossRegistries(t *testing.T) {
	r, target, a := fixture(t, "aws", "worker")
	target.Region, target.Container = "us-east-2", "123456789012"
	WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.json"), target)
	source := target
	source.Environment = "stg"
	WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/stg.json"), source)
	manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json"}`)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
	a.Image = "123456789012.dkr.ecr.us-east-2.amazonaws.com/target@" + digest
	a.SourceImage = "123456789012.dkr.ecr.us-east-2.amazonaws.com/source@" + digest
	a.SourceEnvironment = "stg"
	WriteJSON(filepath.Join(r.Root, "releases/dev.json"), a)
	copied := false
	r.Run = func(_ context.Context, name string, args []string, _ []byte, _ ...string) ([]byte, error) {
		if name == "aws" {
			return []byte("temporary-test-token"), nil
		}
		if name != "skopeo" {
			t.Fatal("unexpected tool")
		}
		switch args[0] {
		case "login":
			return nil, nil
		case "copy":
			if !strings.Contains(strings.Join(args, " "), "--all --preserve-digests") {
				t.Fatal("copy can rewrite digests")
			}
			if args[len(args)-2] != "docker://"+a.SourceImage {
				t.Fatal("wrong source")
			}
			copied = true
			return nil, nil
		case "inspect":
			if !copied {
				return nil, errors.New("image not yet present")
			}
			return manifest, nil
		default:
			t.Fatal("unexpected command")
			return nil, nil
		}
	}
	if err := r.Stage(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
	// Once staged, a rollback must not need the old source environment.
	os.Remove(filepath.Join(r.Root, ".neckbeard/deploy/stg.json"))
	if err := r.Stage(context.Background(), "dev"); err != nil {
		t.Fatal("already-staged digest still required source access:", err)
	}
	a.SourceImage = nextImage
	if a.Validate(Target{App: a.App, Environment: a.Environment}) == nil {
		t.Fatal("promotion accepted a different source digest")
	}
}

func TestRegistryLoginKeepsCredentialsOutOfArguments(t *testing.T) {
	for _, tc := range []struct{ cloud, image, response string }{
		{"aws", "123456789012.dkr.ecr.us-east-2.amazonaws.com/app", "temporary-test-token"},
		{"gcp", "us-east-2-docker.pkg.dev/project/registry/app", "temporary-test-token"},
		{"azure", "testregistry.azurecr.io/app", `{"accessToken":"temporary-test-token","loginServer":"testregistry.azurecr.io"}`},
	} {
		t.Run(tc.cloud, func(t *testing.T) {
			target := Target{Cloud: tc.cloud, Region: "us-east-2", Container: "project"}
			if tc.cloud == "aws" {
				target.Container = "123456789012"
			}
			logins := 0
			r := Runner{Run: func(_ context.Context, name string, args []string, input []byte, _ ...string) ([]byte, error) {
				if strings.Contains(strings.Join(args, " "), "temporary-test-token") {
					t.Fatal("credential exposed in process arguments")
				}
				if name == "skopeo" {
					if string(input) != "temporary-test-token" || !strings.Contains(strings.Join(args, " "), "--password-stdin") {
						t.Fatal("credential not passed through stdin")
					}
					logins++
					return nil, nil
				}
				return []byte(tc.response), nil
			}}
			if err := r.registryLogin(context.Background(), target, tc.image, "private-auth.json"); err != nil {
				t.Fatal(err)
			}
			if logins != 1 {
				t.Fatal("registry was not authenticated")
			}
			r.Run = func(context.Context, string, []string, []byte, ...string) ([]byte, error) {
				t.Fatal("credential requested for an untrusted registry")
				return nil, nil
			}
			if err := r.registryLogin(context.Background(), target, "untrusted.example/app", "private-auth.json"); err == nil {
				t.Fatal("untrusted registry accepted")
			}
		})
	}
}

func TestCIOutputsAreNeverShellEvaluated(t *testing.T) {
	r, target, _ := fixture(t, "aws", "worker")
	target.VCS, target.Repo = "github", "owner/test"
	payload := "$(touch /tmp/should-not-exist); quoted value"
	calls := 0
	r.Run = func(_ context.Context, name string, args []string, input []byte, _ ...string) ([]byte, error) {
		if name == "tofu" {
			if args[len(args)-1] == "github_subject_prefix" {
				return []byte("repo:owner@123/test@456"), nil
			}
			if args[len(args)-1] == "ci_variables" {
				return json.Marshal(map[string]string{"NECKBEARD_AWS_APPLY_ROLE_DEV": payload})
			}
			if args[len(args)-1] == "release_store" {
				return []byte("s3://test-state-bucket"), nil
			}
			return []byte("registry.example/app"), nil
		}
		if name != "gh" {
			t.Fatal("unexpected executable", name)
		}
		if len(args) == 2 && args[0] == "api" {
			return []byte(`{"sub_claim_prefix":"repo:owner@123/test@456"}`), nil
		}
		if args[0] == "variable" {
			if strings.Contains(strings.Join(args, " "), payload) {
				t.Fatal("variable value placed in argv")
			}
			if args[2] == "NECKBEARD_AWS_APPLY_ROLE_DEV" && string(input) != payload {
				t.Fatal("variable value altered")
			}
			calls++
		}
		return []byte(`{}`), nil
	}
	if err := r.ConfigureCI(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("published %d variables", calls)
	}
}

func TestCIRejectsUnverifiedGitHubSubjectBeforePublishing(t *testing.T) {
	for _, cloud := range []string{"aws", "azure"} {
		for _, response := range []string{
			`{"sub_claim_prefix":"repo:owner/test"}`,
			`{"sub_claim_prefix":"repo:owner@123/test@999"}`,
			`{}`,
			`not-json`,
		} {
			t.Run(cloud+"/"+response, func(t *testing.T) {
				r, target, _ := fixture(t, cloud, "worker")
				target.VCS, target.Repo = "github", "owner/test"
				r.Run = func(_ context.Context, name string, args []string, _ []byte, _ ...string) ([]byte, error) {
					if name == "tofu" && args[len(args)-1] == "github_subject_prefix" {
						return []byte("repo:owner@123/test@456"), nil
					}
					if name == "gh" && len(args) == 2 && args[0] == "api" {
						return []byte(response), nil
					}
					t.Fatalf("CI setup advanced before verifying identity: %s %v", name, args)
					return nil, nil
				}
				if err := r.ConfigureCI(context.Background(), target); err == nil || !strings.Contains(err.Error(), "subject prefix differs") {
					t.Fatalf("expected subject mismatch, got %v", err)
				}
			})
		}
	}
}

func TestProductionGate(t *testing.T) {
	for _, tc := range []struct {
		name, vcs, response string
		allowed             bool
	}{
		{"github unavailable on plan", "github", `{"protection_rules":[]}`, false},
		{"github self approval", "github", `{"protection_rules":[{"type":"required_reviewers","prevent_self_review":false,"reviewers":[{}]}]}`, false},
		{"github empty reviewers", "github", `{"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[]}]}`, false},
		{"github independent approval", "github", `{"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User"}]}]}`, true},
		{"gitlab protected without approval", "gitlab", `{"name":"prd","required_approval_count":0,"approval_rules":[]}`, false},
		{"gitlab current multiple rules", "gitlab", `{"name":"prd","required_approval_count":0,"approval_rules":[{"required_approvals":1}]}`, true},
		{"gitlab legacy count", "gitlab", `{"name":"prd","required_approval_count":1}`, true},
		{"gitlab wrong environment", "gitlab", `{"name":"dev","required_approval_count":1}`, false},
		{"invalid response", "github", `not json`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, target, _ := fixture(t, "gcp", "worker")
			target.VCS, target.Repo, target.Environment = tc.vcs, "owner/test", "prd"
			r.Run = func(_ context.Context, name string, args []string, _ []byte, _ ...string) ([]byte, error) {
				if (name != "gh" && name != "glab") || len(args) != 2 || args[0] != "api" {
					t.Fatalf("unexpected mutation or bootstrap read: %s %v", name, args)
				}
				return []byte(tc.response), nil
			}
			err := r.checkProductionGate(context.Background(), target)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, got %v", tc.allowed, err)
			}
			if !tc.allowed {
				if err := r.ConfigureCI(context.Background(), target); err == nil {
					t.Fatal("published production configuration without an approval gate")
				}
			}
		})
	}
}

func TestRollbackUnverifiedUpdateUsesLastSuccessfulRelease(t *testing.T) {
	for _, history := range []bool{false, true} {
		t.Run(fmt.Sprint("older-history=", history), func(t *testing.T) {
			r, _, good := fixture(t, "aws", "worker")
			receipt := Receipt{good, time.Now().UTC().Format(time.RFC3339)}
			if err := WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.receipt.json"), receipt); err != nil {
				t.Fatal(err)
			}
			if history {
				older := receipt
				older.Artifact.Image = "registry.example/app@sha256:" + strings.Repeat("c", 64)
				if err := WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.previous.json"), older); err != nil {
					t.Fatal(err)
				}
			}
			// A failed update leaves its requested image in release intent, while
			// the current receipt still names the last successful deployment.
			failed := good
			failed.Image = nextImage
			if err := WriteJSON(filepath.Join(r.Root, "releases/dev.json"), failed); err != nil {
				t.Fatal(err)
			}
			if err := Main([]string{"rollback", "-root", r.Root, "-env", "dev"}); err != nil {
				t.Fatal(err)
			}
			var intent Artifact
			if err := ReadJSON(filepath.Join(r.Root, "releases/dev.json"), &intent); err != nil {
				t.Fatal(err)
			}
			if intent.Image != good.Image {
				t.Fatalf("rollback skipped the last successful release: got %s, want %s", intent.Image, good.Image)
			}
		})
	}
}
