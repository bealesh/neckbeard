// Package skillinstall places the neckbeard skill where coding agents discover
// it. SKILL.md is a cross-agent standard; targets differ only in path
// conventions (and Cursor additionally supports repo-level slash commands).
package skillinstall

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pluginassets "github.com/bealesh/neckbeard/plugin"
)

// Target is a tool's skill-discovery convention.
type Target string

const (
	// TargetAgents is the tool-neutral open-standard location, honored by Codex
	// CLI, Cursor, and others.
	TargetAgents Target = "agents"
	TargetCodex  Target = "codex"
	TargetCursor Target = "cursor"
)

// skillDir returns the directory (relative to root) that should contain
// SKILL.md for the target.
func skillDir(t Target) (string, error) {
	switch t {
	case TargetAgents:
		return filepath.Join(".agents", "skills", "neckbeard"), nil
	case TargetCodex:
		return filepath.Join(".codex", "skills", "neckbeard"), nil
	case TargetCursor:
		return filepath.Join(".cursor", "skills", "neckbeard"), nil
	default:
		return "", fmt.Errorf("unknown target %q (agents|codex|cursor; Claude Code installs via its plugin marketplace)", t)
	}
}

// Install writes the skill under root (a project directory, or a home directory
// for user-scoped installs) and returns the paths written. Existing files are
// overwritten: the skill is generated tool guidance, not user work product.
func Install(root string, t Target) ([]string, error) {
	dir, err := skillDir(t)
	if err != nil {
		return nil, err
	}
	var written []string
	write := func(rel string, content []byte) error {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			return err
		}
		written = append(written, rel)
		return nil
	}

	if err := write(filepath.Join(dir, "SKILL.md"), pluginassets.SkillMD); err != nil {
		return nil, err
	}

	// Cursor also supports repo-level slash commands; the Claude command bodies
	// port directly once their claude-plugin frontmatter is stripped.
	if t == TargetCursor {
		if err := write(filepath.Join(".cursor", "commands", "neckbeard-analyze.md"), commandBody(pluginassets.AnalyzeCommand)); err != nil {
			return nil, err
		}
		if err := write(filepath.Join(".cursor", "commands", "neckbeard-sync.md"), commandBody(pluginassets.SyncCommand)); err != nil {
			return nil, err
		}
	}
	return written, nil
}

var frontmatterRe = regexp.MustCompile(`(?s)\A---\n.*?\n---\n`)

// commandBody strips the plugin-command frontmatter and the Claude-specific
// $ARGUMENTS placeholder, leaving a plain prompt file.
func commandBody(md []byte) []byte {
	body := frontmatterRe.ReplaceAll(md, nil)
	body = []byte(strings.ReplaceAll(string(body), "$ARGUMENTS", ""))
	return []byte(strings.TrimSpace(string(body)) + "\n")
}
