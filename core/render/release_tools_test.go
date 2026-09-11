package render

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/catalog"
)

// The generated install script runs in jobs that later hold deployment
// credentials: every download must verify against a recorded checksum, and a
// tool bump without its checksum must fail rendering, not ship unverified.
func TestReleaseToolDownloadsArePinnedByChecksum(t *testing.T) {
	cat, err := catalog.Load(filepath.Join("..", "..", "catalog", "index.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if opentofuSHA256[cat.OpenTofu] == "" {
		t.Fatalf("catalog pins OpenTofu %s but opentofuSHA256 has no checksum for it", cat.OpenTofu)
	}

	for _, lane := range []struct{ cloud, runtime string }{
		{"aws", "serverless-containers"},
		{"gcp", "serverless-containers"},
		{"azure", "serverless-containers"},
		{"aws", "kubernetes"},
		{"gcp", "kubernetes"},
		{"azure", "kubernetes"},
	} {
		bp := testBlueprint(t)
		bp.Cloud, bp.Runtime = lane.cloud, lane.runtime
		script := string(releaseTools(bp))
		if strings.Contains(script, "_VERSION") || strings.Contains(script, "_SHA256") {
			t.Errorf("%s/%s: script has an unsubstituted placeholder", lane.cloud, lane.runtime)
		}
		for _, line := range strings.Split(script, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "curl ") && !strings.Contains(script, "fetch() {") {
				t.Fatalf("%s/%s: fetch helper missing yet curl present", lane.cloud, lane.runtime)
			}
			// Every download must go through fetch (which verifies); the only raw
			// curl allowed is the one inside the fetch helper itself.
			if strings.HasPrefix(trimmed, "curl ") && !strings.Contains(trimmed, `"$1" -o "$2"`) {
				t.Errorf("%s/%s: unverified download: %s", lane.cloud, lane.runtime, trimmed)
			}
			if strings.HasPrefix(trimmed, "fetch ") && !strings.Contains(trimmed, "https://") {
				t.Errorf("%s/%s: fetch without an https source: %s", lane.cloud, lane.runtime, trimmed)
			}
		}
		// Each fetch call's third argument must be a 64-hex checksum.
		for _, line := range strings.Split(script, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "fetch \"https://") {
				continue
			}
			fields := strings.Fields(trimmed)
			last := strings.Trim(fields[len(fields)-1], `"`)
			if len(last) != 64 || strings.Trim(last, "0123456789abcdef") != "" {
				t.Errorf("%s/%s: fetch without a sha256 pin: %s", lane.cloud, lane.runtime, trimmed)
			}
		}
	}
}

func TestUnknownOpenTofuVersionRefusesToRender(t *testing.T) {
	bp := testBlueprint(t)
	bp.Pins.OpenTofu = "9.9.9"
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "opentofuSHA256") {
			t.Fatalf("expected a panic naming opentofuSHA256, got %v", r)
		}
	}()
	releaseTools(bp)
}
