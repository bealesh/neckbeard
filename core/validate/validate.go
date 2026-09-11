// Package validate implements the V0 static level of the validation ladder
// (DESIGN §12.1). It reports exactly what ran: PASSED / FAILED per check, and
// NOT EXERCISED for everything this level cannot prove — offline checks never
// imply deployability or runtime behavior.
package validate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type actionlintIssue struct {
	Message  string `json:"message"`
	Filepath string `json:"filepath"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Kind     string `json:"kind"`
}

// filterActionlint drops exactly one known-false diagnostic — the installed
// actionlint not yet recognizing GitHub's supported concurrency `queue` key
// (github.com/rhysd/actionlint/issues/680) — and only when the flagged line
// literally reads `queue: max`. Every other diagnostic is kept; a report that
// can't be parsed is not filtered at all.
func filterActionlint(root string, raw []byte) (kept []actionlintIssue, excluded int, err error) {
	var issues []actionlintIssue
	if err := json.Unmarshal(raw, &issues); err != nil {
		return nil, 0, fmt.Errorf("unparseable actionlint output: %w", err)
	}
	for _, issue := range issues {
		if issue.Kind == "syntax-check" &&
			strings.Contains(issue.Message, `unexpected key "queue" for "concurrency" section`) &&
			fileLine(filepath.Join(root, issue.Filepath), issue.Line) == "queue: max" {
			excluded++
			continue
		}
		kept = append(kept, issue)
	}
	return kept, excluded, nil
}

func fileLine(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for i := 1; scanner.Scan(); i++ {
		if i == n {
			return strings.TrimSpace(scanner.Text())
		}
	}
	return ""
}

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
	roots := make([]struct{ env, dir string }, 0, len(envs)*2)
	for _, env := range envs {
		roots = append(roots,
			struct{ env, dir string }{env, filepath.Join(root, "infra", "envs", env)},
			struct{ env, dir string }{env + " bootstrap", filepath.Join(root, "infra", "bootstrap", env)},
		)
	}
	for _, r := range roots {
		env, dir := r.env, r.dir
		if _, statErr := os.Stat(dir); statErr != nil {
			if strings.Contains(env, "bootstrap") {
				continue // bootstrap roots are optional in older scaffolds
			}
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

	checks = append(checks, pipelineChecks(root)...)
	checks = append(checks, policyChecks(root, envs)...)
	checks = append(checks, manifestChecks(root)...)

	// What this run did NOT prove, stated instead of implied (DESIGN §12.1).
	checks = append(checks,
		Check{Level: "V1", Name: "authenticated plan", Status: NotExercised, Detail: "requires cloud credentials"},
		Check{Level: "V2", Name: "deployment verification", Status: NotExercised, Detail: "release-harness only for now"},
	)
	return checks, nil
}

// policyChecks runs checkov over each env root. Skips live in the generated
// .checkov.yaml, each with a written reason — a skip without a reason is a lie
// about the security posture.
func policyChecks(root string, envs []string) []Check {
	checkov, err := exec.LookPath("checkov")
	if err != nil {
		return []Check{{Level: "V0", Name: "policy checks (checkov)", Status: NotExercised, Detail: "checkov not installed (`brew install checkov` / pipx install checkov)"}}
	}
	var checks []Check
	dirs := make([]struct{ env, dir string }, 0, len(envs)*2)
	for _, env := range envs {
		dirs = append(dirs,
			struct{ env, dir string }{env, filepath.Join(root, "infra", "envs", env)},
			struct{ env, dir string }{env + " bootstrap", filepath.Join(root, "infra", "bootstrap", env)},
		)
	}
	for _, d := range dirs {
		env, dir := d.env, d.dir
		if _, statErr := os.Stat(dir); statErr != nil {
			continue // the missing root is already reported by the tofu steps
		}
		args := []string{"-d", dir, "--quiet", "--compact", "--framework", "terraform"}
		if _, cfgErr := os.Stat(filepath.Join(root, ".checkov.yaml")); cfgErr == nil {
			args = append(args, "--config-file", filepath.Join(root, ".checkov.yaml"))
		}
		cmd := exec.Command(checkov, args...)
		cmd.Dir = root
		out, runErr := cmd.CombinedOutput()
		c := Check{Level: "V0", Name: "checkov policy", Env: env, Status: Passed}
		if runErr != nil {
			c.Status = Failed
			c.Detail = lastLines(string(out), 8)
		}
		checks = append(checks, c)
	}
	return checks
}

// pipelineChecks lints generated CI files with what is locally available and is
// explicit about what is not: GitLab's CI lint needs a GitLab instance.
func pipelineChecks(root string) []Check {
	var checks []Check
	if workflows, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "neckbeard-*.yml")); err == nil && len(workflows) > 0 {
		if actionlint, lookErr := exec.LookPath("actionlint"); lookErr == nil {
			cmd := exec.Command(actionlint, append([]string{"-format", "{{json .}}"}, workflows...)...)
			cmd.Dir = root
			out, runErr := cmd.CombinedOutput()
			c := Check{Level: "V0", Name: "actionlint (GitHub workflows)", Status: Passed}
			if runErr != nil {
				kept, excluded, parseErr := filterActionlint(root, out)
				switch {
				case parseErr != nil:
					c.Status = Failed
					c.Detail = lastLines(string(out), 6)
				case len(kept) > 0:
					c.Status = Failed
					lines := make([]string, 0, len(kept))
					for _, issue := range kept {
						lines = append(lines, fmt.Sprintf("%s:%d:%d: %s [%s]", issue.Filepath, issue.Line, issue.Column, issue.Message, issue.Kind))
					}
					c.Detail = lastLines(strings.Join(lines, "\n"), 6)
				default:
					c.Detail = fmt.Sprintf("%d diagnostic(s) excluded: installed actionlint does not yet recognize GitHub's supported concurrency `queue` key (github.com/rhysd/actionlint/issues/680); each excluded line reads exactly `queue: max`", excluded)
				}
			}
			checks = append(checks, c)
		} else {
			checks = append(checks, Check{Level: "V0", Name: "actionlint (GitHub workflows)", Status: NotExercised, Detail: "actionlint not installed"})
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".gitlab-ci.yml")); err == nil {
		checks = append(checks, Check{Level: "V0", Name: "GitLab CI lint", Status: NotExercised, Detail: "requires a GitLab instance's CI lint API"})
	}
	return checks
}

// manifestChecks validates the kubernetes delivery layer (clusters/) with
// kubeconform: core schemas plus the community CRD catalog for Flux resources.
func manifestChecks(root string) []Check {
	clustersDir := filepath.Join(root, "clusters")
	if _, err := os.Stat(clustersDir); err != nil {
		return nil // not a kubernetes lane
	}
	kubeconform, err := exec.LookPath("kubeconform")
	if err != nil {
		return []Check{{Level: "V0", Name: "kubeconform (clusters/)", Status: NotExercised, Detail: "kubeconform not installed (`brew install kubeconform`)"}}
	}
	cmd := exec.Command(kubeconform,
		"-strict", "-summary",
		"-schema-location", "default",
		"-schema-location", "https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json",
		clustersDir)
	out, runErr := cmd.CombinedOutput()
	c := Check{Level: "V0", Name: "kubeconform (clusters/)", Status: Passed}
	if runErr != nil {
		c.Status = Failed
		c.Detail = lastLines(string(out), 8)
	}
	return []Check{c}
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
