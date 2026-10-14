package skillinstall

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pluginassets "github.com/bealesh/neckbeard"
)

func TestEmbeddedAssetsAreReal(t *testing.T) {
	if !strings.Contains(string(pluginassets.SkillMD), "NEVER hand-write infrastructure") {
		t.Error("embedded SKILL.md missing the prime directive — wrong file embedded?")
	}
	if !strings.HasPrefix(string(pluginassets.SkillMD), "---\n") {
		t.Error("SKILL.md must keep its frontmatter (the standard requires name/description)")
	}
}

func TestInstallTargets(t *testing.T) {
	cases := map[Target]string{
		TargetAgents: ".agents/skills/neckbeard/SKILL.md",
		TargetCodex:  ".codex/skills/neckbeard/SKILL.md",
		TargetCursor: ".cursor/skills/neckbeard/SKILL.md",
	}
	for target, want := range cases {
		root := t.TempDir()
		written, err := Install(root, target)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
			t.Errorf("%s: expected %s: %v", target, want, err)
		}
		if len(written) == 0 {
			t.Errorf("%s: no paths reported", target)
		}
	}
}

func TestCursorGetsCommands(t *testing.T) {
	root := t.TempDir()
	if _, err := Install(root, TargetCursor); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".cursor", "commands", "neckbeard-analyze.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.HasPrefix(s, "---") {
		t.Error("cursor commands must not carry claude-plugin frontmatter")
	}
	if strings.Contains(s, "$ARGUMENTS") {
		t.Error("cursor commands must not carry the $ARGUMENTS placeholder")
	}
	if !strings.Contains(s, "neckbeard analyze") {
		t.Error("command body lost its content")
	}
}

func TestUnknownTargetRefused(t *testing.T) {
	if _, err := Install(t.TempDir(), Target("vscode")); err == nil || !strings.Contains(err.Error(), "plugin marketplace") {
		t.Errorf("unknown targets need a helpful refusal, got: %v", err)
	}
}

func TestEveryCommandHasFrontmatter(t *testing.T) {
	entries, err := fs.ReadDir(pluginassets.Commands, "commands")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected the plugin commands to be embedded, got %d entries", len(entries))
	}
	for _, e := range entries {
		md, err := pluginassets.Commands.ReadFile("commands/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		s := string(md)
		if !strings.HasPrefix(s, "---\n") || !strings.Contains(s, "\ndescription:") {
			t.Errorf("%s: plugin commands need frontmatter with a description", e.Name())
		}
		if !strings.Contains(s, "\n---\n") {
			t.Errorf("%s: unterminated frontmatter", e.Name())
		}
	}
}
