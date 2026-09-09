// Package doctor checks local prerequisites without contacting cloud accounts.
package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Check struct{ Tool, Version, Status, Detail string }

// Infracost locates the supported classic CLI before the unrelated v2 CLI.
func Infracost(explicit string) (string, error) {
	if explicit != "" {
		return exec.LookPath(explicit)
	}
	if bin, err := exec.LookPath("infracost-0.10"); err == nil {
		return bin, nil
	}
	return exec.LookPath("infracost")
}

var versionRE = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

func Version(bin string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s --version failed: %w", bin, err)
	}
	v := versionRE.FindString(string(out))
	if v == "" {
		return "", fmt.Errorf("%s returned no recognizable version", bin)
	}
	return v, nil
}

func CheckTools(stage, tofuVersion string) ([]Check, error) {
	if stage != "plan" && stage != "estimate" && stage != "validate" && stage != "all" {
		return nil, fmt.Errorf("-for must be plan, estimate, validate, or all")
	}
	var checks []Check
	for _, spec := range []struct{ name, stage, tested, hint string }{
		{"infracost", "estimate", "0.10.", "install Infracost 0.10.x (https://github.com/infracost/infracost/releases), then run infracost auth login"},
		{"tofu", "validate", tofuVersion, "install the pinned OpenTofu version from https://github.com/opentofu/opentofu/releases"},
		{"checkov", "validate", "", "install Checkov (https://www.checkov.io); generated CI uses 3.3.10"},
		{"kubeconform", "validate", "", "install kubeconform for Kubernetes checks (https://github.com/yannh/kubeconform)"},
		{"actionlint", "validate", "", "install actionlint for GitHub workflow checks (https://github.com/rhysd/actionlint)"},
	} {
		if stage != "all" && stage != spec.stage {
			continue
		}
		bin, err := exec.LookPath(spec.name)
		if spec.name == "infracost" {
			bin, err = Infracost("")
		}
		c := Check{Tool: spec.name, Status: "READY"}
		if err != nil {
			c.Status, c.Detail = "MISSING", spec.hint
		} else if spec.tested != "" {
			c.Version, err = Version(bin)
			if err != nil {
				c.Status, c.Detail = "FAILED", err.Error()
			} else if (spec.name == "tofu" && c.Version != spec.tested) || (spec.name == "infracost" && !strings.HasPrefix(c.Version, spec.tested)) {
				c.Status, c.Detail = "INCOMPATIBLE", "expected "+spec.tested+"; "+spec.hint
			}
		} else {
			c.Detail = bin
		}
		checks = append(checks, c)
	}
	return checks, nil
}
