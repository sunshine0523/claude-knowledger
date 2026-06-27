---
name: update-knowledger
description: Use when the user says "更新knowledger", "update knowledger", "升级knowledger", "刷新knowledger", or wants to update and reinstall the knowledger CLI and Claude Code plugin.
version: 1.0.0
triggers:
  - "更新knowledger"
  - "升级knowledger"
  - "刷新knowledger"
  - "update knowledger"
  - "update the knowledger"
  - "rebuild knowledger"
  - "reinstall knowledger"
  - "更新claude code插件"
  - "reinstall plugin"
---

# Update Knowledger

Updates the knowledger binary via the official install script and reinstalls the Claude Code plugin.

## Workflow

```dot
digraph update_flow {
  "User says 更新knowledger" [shape=doublecircle];
  "Detect OS" [shape=diamond];
  "curl install.sh (Linux/macOS)" [shape=box];
  "irm install.ps1 (Windows)" [shape=box];
  "knowledger install --claude" [shape=box];
  "Verify: knowledger --version" [shape=box];
  "Done" [shape=doublecircle];

  "User says 更新knowledger" -> "Detect OS";
  "Detect OS" -> "curl install.sh (Linux/macOS)" [label="Linux / macOS"];
  "Detect OS" -> "irm install.ps1 (Windows)" [label="Windows"];
  "curl install.sh (Linux/macOS)" -> "knowledger install --claude";
  "irm install.ps1 (Windows)" -> "knowledger install --claude";
  "knowledger install --claude" -> "Verify: knowledger --version";
  "Verify: knowledger --version" -> "Done";
}
```

### Step 1: Install/update binary via official script

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/kindbrave/claude-knowledger/main/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/kindbrave/claude-knowledger/main/install.ps1 | iex
```

The script auto-detects OS and architecture, downloads the latest release from GitHub, and installs to `/usr/local/bin` (or `~/.local/bin` if not writable).

### Step 2: Reinstall Claude Code plugin

```bash
knowledger install --claude
```

### Step 3: Verify

```bash
knowledger --version
```

Report the version and confirm plugin is active.

## Error Handling

- If `curl`/`irm` fails → check network connectivity, verify GitHub is reachable
- If `knowledger install --claude` fails → report, suggest `claude plugin validate ./plugins/claude-code-knowledger`
