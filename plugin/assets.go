// Package pluginassets embeds the agent-facing assets so the CLI binary is the
// single distribution channel: `neckbeard skill install` places the SKILL.md —
// the cross-agent open standard adopted by Codex CLI, Cursor, Gemini CLI, and
// others — where the user's tool discovers it. Claude Code users get the same
// content through the plugin marketplace instead.
package pluginassets

import "embed"

//go:embed skills/neckbeard/SKILL.md
var SkillMD []byte

//go:embed commands/analyze.md
var AnalyzeCommand []byte

//go:embed commands/sync.md
var SyncCommand []byte

//go:embed skills/neckbeard/references
var References embed.FS
