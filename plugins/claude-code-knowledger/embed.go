package claudecodeknowledger

import "embed"

//go:embed README.md .mcp.json .claude-plugin/plugin.json .claude-plugin/marketplace.json skills/knowledger/SKILL.md
var Bundle embed.FS
