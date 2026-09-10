package pipeline

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/blueprint"
	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "rewrite golden files")

func testModel(t *testing.T) Model {
	t.Helper()
	m, err := Build(&blueprint.Blueprint{
		App:    "bellwether",
		Region: "us-east-1",
		Pins:   blueprint.Pins{OpenTofu: "1.12.6"},
		Services: []blueprint.Service{
			{Name: "web", Kind: "http", Port: 8080, HealthPath: "/healthz", Dockerfile: "bellwether/Dockerfile"},
			{Name: "worker", Kind: "worker", Dockerfile: "bellwether/Dockerfile"},
		},
		Environments: []blueprint.Environment{{Name: "dev"}, {Name: "stg"}, {Name: "prd"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Golden tests prove rendering determinism and reviewable output only — execution
// behavior (auth, artifacts, promotion) is verified by the release harness
// (DESIGN §11.1, §12).
func TestGoldenRenderings(t *testing.T) {
	m := testModel(t)
	cases := []struct {
		golden string
		got    []byte
	}{
		{"github-ci.golden.yml", RenderGitHubCI(m)},
		{"github-infra.golden.yml", RenderGitHubInfra(m)},
		{"github-release.golden.yml", RenderGitHubRelease(m)},
		{"gitlab-ci.golden.yml", RenderGitLab(m)},
	}
	for _, c := range cases {
		path := filepath.Join("testdata", c.golden)
		if *update {
			if err := os.MkdirAll("testdata", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, c.got, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s (run `make golden-update`?): %v", c.golden, err)
		}
		if !bytes.Equal(c.got, want) {
			t.Errorf("%s differs from golden.\n--- got ---\n%s", c.golden, c.got)
		}
	}
}

func TestRenderingsAreValidYAML(t *testing.T) {
	m := testModel(t)
	for name, content := range map[string][]byte{
		"github-ci":      RenderGitHubCI(m),
		"github-infra":   RenderGitHubInfra(m),
		"github-release": RenderGitHubRelease(m),
		"gitlab-ci":      RenderGitLab(m),
	} {
		var v any
		if err := yaml.Unmarshal(content, &v); err != nil {
			t.Errorf("%s is not valid YAML: %v", name, err)
		}
	}
}

func TestNoStaticCloudKeys(t *testing.T) {
	m := testModel(t)
	for name, content := range map[string][]byte{
		"github-ci":      RenderGitHubCI(m),
		"github-infra":   RenderGitHubInfra(m),
		"github-release": RenderGitHubRelease(m),
		"gitlab-ci":      RenderGitLab(m),
	} {
		s := string(content)
		for _, banned := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
			if strings.Contains(s, banned) {
				t.Errorf("%s references %s — pipelines must use OIDC federation only (DESIGN §10.1)", name, banned)
			}
		}
	}
}

func TestPrdIsGatedOnGitLab(t *testing.T) {
	s := string(RenderGitLab(testModel(t)))
	prdIdx := strings.Index(s, "infra-prd:")
	if prdIdx == -1 {
		t.Fatal("no infra-prd job")
	}
	if !strings.Contains(s[prdIdx:], "when: manual") {
		t.Error("prd apply must be a manual, environment-bound job (§11.3)")
	}
	stgIdx := strings.Index(s, "infra-stg:")
	if strings.Contains(s[stgIdx:prdIdx], "when: manual") {
		t.Error("stg must not be manual")
	}
}

func TestMultipleDockerfilesRefused(t *testing.T) {
	_, err := Build(&blueprint.Blueprint{
		App:  "x",
		Pins: blueprint.Pins{OpenTofu: "1.12.6"},
		Services: []blueprint.Service{
			{Name: "a", Kind: "http", Dockerfile: "a/Dockerfile"},
			{Name: "b", Kind: "worker", Dockerfile: "b/Dockerfile"},
		},
		Environments: []blueprint.Environment{{Name: "dev"}},
	})
	if err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Errorf("expected named refusal for multi-image apps, got: %v", err)
	}
}
