package claudecodeknowledger

import "embed"

//go:embed README.md .mcp.json .claude-plugin/plugin.json .claude-plugin/marketplace.json skills/knowledger/SKILL.md skills/git-knowledge/SKILL.md hooks/hooks.json hooks/precheck hooks/git-sync
var Bundle embed.FS
