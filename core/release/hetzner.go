package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Hetzner has no deployment API. The server is a box, so the runner does over
// SSH exactly what docs/bootstrap.md tells an operator to do by hand: rewrite
// /etc/neckbeard/image.env and reload neckbeard-app.service, whose reload
// converges docker compose onto the new image without touching Postgres. It
// then checks docker's own account of each service container rather than
// trusting the exit code of the reload.
//
// Trust is fail-closed in both directions. There is no cloud API that returns
// the server's host key, so the key must be committed to
// .neckbeard/deploy/<env>.known_hosts and host checking is strict; and the
// image written is the digest-pinned reference the artifact already validated,
// passed on stdin so nothing is shell-evaluated on the way.

const hetznerIdentityEnv = "NECKBEARD_SSH_IDENTITY"

func (r Runner) hetzner(ctx context.Context, t Target, a Artifact, values map[string]json.RawMessage) error {
	host, err := outputString(values, "server_ipv4")
	if err != nil {
		return err
	}
	var names []string
	if err := json.Unmarshal(values["service_names"], &names); err != nil || len(names) == 0 {
		return fmt.Errorf("deployment output \"service_names\" is missing; complete infrastructure setup first")
	}
	knownHosts := filepath.Join(r.Root, ".neckbeard/deploy", t.Environment+".known_hosts")
	if _, err := os.Stat(knownHosts); err != nil {
		return fmt.Errorf("no committed host key for the %s server: after checking the fingerprint against the server console, run `ssh-keyscan -t ed25519 %s > %s` and commit the file (docs/bootstrap.md)", t.Environment, host, knownHosts)
	}
	ssh := func(script string) ([]byte, error) {
		args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + knownHosts, "-o", "ConnectTimeout=20"}
		if identity := os.Getenv(hetznerIdentityEnv); identity != "" {
			args = append(args, "-o", "IdentitiesOnly=yes", "-i", identity)
		}
		args = append(args, "root@"+host, "sh", "-eu")
		return r.Run(ctx, "ssh", args, []byte(script))
	}
	update := strings.Join([]string{
		`test -f /etc/neckbeard/app.env || { echo "neckbeard-app has never been started: an operator must write /etc/neckbeard/app.env and start the unit once (docs/bootstrap.md)" >&2; exit 64; }`,
		`systemctl is-active --quiet neckbeard-app.service || { echo "neckbeard-app.service is not active; inspect the server before releasing" >&2; exit 64; }`,
		`umask 022`,
		`printf 'APP_IMAGE=%s\n' '` + a.Image + `' > /etc/neckbeard/image.env.next`,
		`mv /etc/neckbeard/image.env.next /etc/neckbeard/image.env`,
		`systemctl reload neckbeard-app.service`,
	}, "\n") + "\n"
	if _, err := ssh(update); err != nil {
		return err
	}
	inspect := "docker ps -aq --filter label=com.docker.compose.project=neckbeard | xargs -r docker inspect\n"
	return waitReady(ctx, 5*60*1e9, func() error {
		out, err := ssh(inspect)
		if err != nil {
			return err
		}
		return hetznerReady(out, t.Services, a.Image)
	})
}

// hetznerReady reads `docker inspect` output for the compose project and
// requires, for every service that runs as a container, exactly one container
// on the released image in the running state, healthy when it serves HTTP.
// Cron services are one-off containers started by cron from image.env, so
// there is nothing of theirs to observe.
func hetznerReady(inspect []byte, services []Service, image string) error {
	var containers []struct {
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status string `json:"Status"`
			Health *struct {
				Status string `json:"Status"`
			} `json:"Health"`
		} `json:"State"`
	}
	if strings.TrimSpace(string(inspect)) == "" {
		return fmt.Errorf("no containers in the neckbeard compose project yet")
	}
	if err := json.Unmarshal(inspect, &containers); err != nil {
		return fmt.Errorf("unreadable docker inspect output: %w", err)
	}
	for _, s := range services {
		if s.Kind == "cron" {
			continue
		}
		found := 0
		for _, c := range containers {
			if c.Config.Labels["com.docker.compose.service"] != s.Name {
				continue
			}
			found++
			if c.Config.Image != image {
				return fmt.Errorf("service %s runs %s, not the released image", s.Name, c.Config.Image)
			}
			if c.State.Status != "running" {
				return fmt.Errorf("service %s is %s", s.Name, c.State.Status)
			}
			if s.Kind == "http" {
				if c.State.Health == nil {
					return fmt.Errorf("service %s has no health check to observe", s.Name)
				}
				if c.State.Health.Status != "healthy" {
					return fmt.Errorf("service %s is %s", s.Name, c.State.Health.Status)
				}
			}
		}
		if found == 0 {
			return fmt.Errorf("service %s has no container", s.Name)
		}
		if found > 1 {
			return fmt.Errorf("service %s has %d containers; compose did not converge", s.Name, found)
		}
	}
	return nil
}
