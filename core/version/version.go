// Package version pins the planner/CLI version recorded in every blueprint.
package version

import (
	"runtime/debug"
	"strings"
)

// fallback is what source builds report; tagged module builds
// (go install …@v0.1.0) report the actual tag so blueprints record exactly
// what produced them.
const fallback = "0.1.0-dev"

func String() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return fallback
}
