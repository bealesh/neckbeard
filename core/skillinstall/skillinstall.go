// Package skillinstall places the neckbeard skill where coding agents discover
// it. SKILL.md is a cross-agent standard; targets differ only in path
// conventions (and Cursor additionally supports repo-level slash commands).
package skillinstall

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pluginassets "github.com/bealesh/neckbeard"
	catalogassets "github.com/bealesh/neckbeard/catalog"
	"github.com/bealesh/neckbeard/core/presets"
	"github.com/bealesh/neckbeard/schemas"
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

	if err := fs.WalkDir(pluginassets.References, "skills/neckbeard/references", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := pluginassets.References.ReadFile(p)
		if err != nil {
			return err
		}
		return write(filepath.Join(dir, "references", filepath.Base(p)), data)
	}); err != nil {
		return nil, err
	}
	for _, name := range []string{"neckbeard", "app-profile", "blueprint", "environment-manifest"} {
		data, err := schemas.Read(name)
		if err != nil {
			return nil, err
		}
		if err := write(filepath.Join(dir, "references", name+".schema.json"), data); err != nil {
			return nil, err
		}
	}
	if err := write(filepath.Join(dir, "references", "presets.yaml"), presets.YAML()); err != nil {
		return nil, err
	}
	index, err := catalogassets.Files.ReadFile("index.yaml")
	if err != nil {
		return nil, err
	}
	if err := write(filepath.Join(dir, "references", "catalog.yaml"), index); err != nil {
		return nil, err
	}

	// Cursor also supports repo-level slash commands; the Claude command bodies
	// port directly once their claude-plugin frontmatter is stripped.
	if t == TargetCursor {
		entries, err := fs.ReadDir(pluginassets.Commands, "commands")
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			md, err := pluginassets.Commands.ReadFile("commands/" + e.Name())
			if err != nil {
				return nil, err
			}
			if err := write(filepath.Join(".cursor", "commands", "neckbeard-"+e.Name()), commandBody(md)); err != nil {
				return nil, err
			}
		}
	}
	return written, nil
}

var frontmatterRe = regexp.MustCompile(`(?s)\A---\n.*?\n---\n`)

// commandBody strips the plugin-command frontmatter and the Claude-specific
// $ARGUMENTS placeholder, and rewrites the /neckbeard:<cmd> namespace to
// Cursor's flat /neckbeard-<cmd> command names, leaving a plain prompt file.
func commandBody(md []byte) []byte {
	body := frontmatterRe.ReplaceAll(md, nil)
	body = []byte(strings.ReplaceAll(string(body), "$ARGUMENTS", ""))
	body = []byte(strings.ReplaceAll(string(body), "/neckbeard:", "/neckbeard-"))
	return []byte(strings.TrimSpace(string(body)) + "\n")
}
