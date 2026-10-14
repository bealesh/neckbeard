// Package neckbeard embeds the agent-facing plugin assets — the skill, its
// references, and the slash commands — so the CLI binary is a complete
// distribution channel on its own: `neckbeard skill install` places them where
// Codex, Cursor, and any SKILL.md-speaking tool discovers them. Claude Code
// users get the same files straight from this repository through the plugin
// marketplace: the repository root is the plugin.
package neckbeard

import "embed"

//go:embed skills/neckbeard/SKILL.md
var SkillMD []byte

// Commands holds every plugin slash command (commands/<name>.md).
//
//go:embed commands
var Commands embed.FS

//go:embed skills/neckbeard/references
var References embed.FS
