package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hetznerFixture(t *testing.T) (Runner, Target, Artifact) {
	t.Helper()
	r, target, a := fixture(t, "hetzner", "worker")
	target.Runtime, target.Region = "vps-containers", "fsn1"
	target.Services = []Service{{Name: "web", Kind: "http", HealthPath: "/healthz"}, {Name: "worker", Kind: "worker"}, {Name: "nightly", Kind: "cron"}}
	a.HealthURL = "" // the HTTPS probe is covered elsewhere; this exercises the SSH path
	target.Services = target.Services[1:]
	if err := WriteJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.json"), target); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(r.Root, "releases/dev.json"), a); err != nil {
		t.Fatal(err)
	}
	return r, target, a
}

func hetznerInspect(image string) string {
	return fmt.Sprintf(`[{"Config":{"Image":%q,"Labels":{"com.docker.compose.service":"web"}},"State":{"Status":"running","Health":{"Status":"healthy"}}},
{"Config":{"Image":%q,"Labels":{"com.docker.compose.service":"worker"}},"State":{"Status":"running"}},
{"Config":{"Image":"postgres:17-alpine","Labels":{"com.docker.compose.service":"postgres"}},"State":{"Status":"running","Health":{"Status":"healthy"}}}]`, image, image)
}

func TestHetznerDeploysOverSSHWithStrictHostKeys(t *testing.T) {
	r, _, a := hetznerFixture(t)
	knownHosts := filepath.Join(r.Root, ".neckbeard/deploy/dev.known_hosts")
	if err := os.WriteFile(knownHosts, []byte("203.0.113.10 ssh-ed25519 AAAA\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(hetznerIdentityEnv, "/tmp/deploy-key")
	var scripts []string
	r.Run = func(_ context.Context, name string, args []string, input []byte, _ ...string) ([]byte, error) {
		switch name {
		case "tofu":
			return []byte(`{"server_ipv4":"203.0.113.10","service_names":["web","worker"]}`), nil
		case "ssh":
			joined := strings.Join(args, " ")
			for _, want := range []string{"BatchMode=yes", "StrictHostKeyChecking=yes", "UserKnownHostsFile=" + knownHosts, "IdentitiesOnly=yes", "-i /tmp/deploy-key", "root@203.0.113.10 sh -eu"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("ssh invocation lacks %q: %s", want, joined)
				}
			}
			if strings.Contains(joined, "sha256:") {
				t.Fatal("the image must travel on stdin, not in the argument list")
			}
			scripts = append(scripts, string(input))
			if len(scripts) == 1 {
				return nil, nil
			}
			return []byte(hetznerInspect(testImage)), nil
		default:
			t.Fatalf("unexpected command %s %v", name, args)
			return nil, nil
		}
	}
	if err := r.Deploy(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 2 {
		t.Fatalf("expected one update and one inspection over SSH, got %d calls", len(scripts))
	}
	update := scripts[0]
	for _, want := range []string{"test -f /etc/neckbeard/app.env", "systemctl is-active --quiet neckbeard-app.service", "APP_IMAGE=%s\\n' '" + testImage + "'", "mv /etc/neckbeard/image.env.next /etc/neckbeard/image.env", "systemctl reload neckbeard-app.service"} {
		if !strings.Contains(update, want) {
			t.Errorf("update script lacks %q:\n%s", want, update)
		}
	}
	if strings.Contains(update, "restart") || strings.Contains(update, "compose down") {
		t.Error("a release must reload, never restart: restarting takes Postgres down with it")
	}
	if !strings.Contains(scripts[1], "label=com.docker.compose.project=neckbeard") {
		t.Errorf("inspection must select the compose project's containers: %s", scripts[1])
	}
	var receipt Receipt
	if err := ReadJSON(filepath.Join(r.Root, ".neckbeard/deploy/dev.receipt.json"), &receipt); err != nil || receipt.Artifact.Image != a.Image {
		t.Fatalf("verified receipt not written: %v %+v", err, receipt)
	}
}

func TestHetznerRefusesWithoutCommittedHostKey(t *testing.T) {
	r, _, _ := hetznerFixture(t)
	r.Run = func(_ context.Context, name string, args []string, _ []byte, _ ...string) ([]byte, error) {
		if name == "ssh" {
			t.Fatal("ssh must not run before the host key is known")
		}
		return []byte(`{"server_ipv4":"203.0.113.10","service_names":["web","worker"]}`), nil
	}
	err := r.Deploy(context.Background(), "dev")
	if err == nil || !strings.Contains(err.Error(), "known_hosts") || !strings.Contains(err.Error(), "ssh-keyscan") {
		t.Fatalf("expected a named refusal pointing at the host key step, got %v", err)
	}
}

func TestHetznerReadinessFromDockerInspect(t *testing.T) {
	services := []Service{{Name: "web", Kind: "http"}, {Name: "worker", Kind: "worker"}, {Name: "nightly", Kind: "cron"}}
	base := hetznerInspect(testImage)
	for _, tc := range []struct {
		name, old, new string
		bad            bool
	}{
		{"ready", "", "", false},
		{"old-image-on-web", fmt.Sprintf(`"Image":%q,"Labels":{"com.docker.compose.service":"web"}`, testImage), fmt.Sprintf(`"Image":%q,"Labels":{"com.docker.compose.service":"web"}`, nextImage), true},
		{"web-unhealthy", `"Health":{"Status":"healthy"}}},` + "\n", `"Health":{"Status":"starting"}}},` + "\n", true},
		{"web-without-healthcheck", `"Status":"running","Health":{"Status":"healthy"}}},` + "\n", `"Status":"running"}},` + "\n", true},
		{"worker-exited", `"worker"}},"State":{"Status":"running"}`, `"worker"}},"State":{"Status":"exited"}`, true},
		{"worker-missing", `"com.docker.compose.service":"worker"`, `"com.docker.compose.service":"other"`, true},
		{"duplicate-worker", `"com.docker.compose.service":"postgres"`, `"com.docker.compose.service":"worker"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := base
			if tc.old != "" {
				if !strings.Contains(raw, tc.old) {
					t.Fatalf("fixture lacks %q", tc.old)
				}
				raw = strings.Replace(raw, tc.old, tc.new, 1)
			}
			err := hetznerReady([]byte(raw), services, testImage)
			if (err != nil) != tc.bad {
				t.Fatalf("readiness error = %v", err)
			}
		})
	}
	if err := hetznerReady([]byte("  \n"), services, testImage); err == nil {
		t.Error("an empty compose project must not count as ready")
	}
	var doc []map[string]any
	if json.Unmarshal([]byte(base), &doc) != nil {
		t.Fatal("fixture is not valid docker inspect JSON")
	}
}
