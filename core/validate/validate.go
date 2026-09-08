// Package validate implements the V0 static level of the validation ladder
// (DESIGN §12.1). It reports exactly what ran: PASSED / FAILED per check, and
// NOT EXERCISED for everything this level cannot prove — offline checks never
// imply deployability or runtime behavior.
package validate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Status string

const (
	Passed       Status = "PASSED"
	Failed       Status = "FAILED"
	NotExercised Status = "NOT EXERCISED"
)

type Check struct {
	Level  string
	Name   string
	Env    string
	Status Status
	Detail string
}

// StaticV0 runs offline checks over the rendered env roots: tofu fmt, init
// (providers only, no backend), and validate. Requires the tofu binary.
func StaticV0(root string, envs []string) ([]Check, error) {
	tofu, err := exec.LookPath("tofu")
	if err != nil {
		return nil, fmt.Errorf("opentofu not found on PATH (install: https://opentofu.org or `brew install opentofu`)")
	}
	cacheDir, err := pluginCacheDir()
	if err != nil {
		return nil, err
	}

	var checks []Check
	for _, env := range envs {
		dir := filepath.Join(root, "infra", "envs", env)
		if _, statErr := os.Stat(dir); statErr != nil {
			checks = append(checks, Check{Level: "V0", Name: "env root exists", Env: env, Status: Failed, Detail: dir + " missing — run `neckbeard scaffold`"})
			continue
		}
		steps := []struct {
			name string
			args []string
		}{
			{"tofu fmt", []string{"fmt", "-check", "-diff"}},
			{"tofu init (providers only)", []string{"init", "-backend=false", "-input=false", "-no-color"}},
			{"tofu validate", []string{"validate", "-no-color"}},
		}
		for _, step := range steps {
			cmd := exec.Command(tofu, step.args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1", "TF_PLUGIN_CACHE_DIR="+cacheDir)
			out, runErr := cmd.CombinedOutput()
			c := Check{Level: "V0", Name: step.name, Env: env, Status: Passed}
			if runErr != nil {
				c.Status = Failed
				c.Detail = lastLines(string(out), 6)
			}
			checks = append(checks, c)
			if runErr != nil {
				break // later steps in this env would only cascade
			}
		}
	}

	// What this run did NOT prove, stated instead of implied (DESIGN §12.1).
	checks = append(checks,
		Check{Level: "V0", Name: "policy checks (checkov/conftest)", Status: NotExercised, Detail: "lands later in M1"},
		Check{Level: "V1", Name: "authenticated plan", Status: NotExercised, Detail: "requires cloud credentials"},
		Check{Level: "V2", Name: "deployment verification", Status: NotExercised, Detail: "release-harness only for now"},
	)
	return checks, nil
}

func AnyFailed(checks []Check) bool {
	for _, c := range checks {
		if c.Status == Failed {
			return true
		}
	}
	return false
}

func pluginCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "neckbeard", "plugin-cache")
	return dir, os.MkdirAll(dir, 0o755)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
