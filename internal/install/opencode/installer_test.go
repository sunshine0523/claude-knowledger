package opencode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallFreshCreatesConfigWithKnowledgerMCPEntry(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	installer := NewInstaller(
		WithExecutablePath(func() (string, error) { return exe, nil }),
		WithHomeDir(func() (string, error) { return home, nil }),
		WithConfigPath(func() (string, error) { return configPath, nil }),
	)
	var out, errOut strings.Builder

	if err := installer.Install(&out, &errOut); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v\nconfig:\n%s", err, data)
	}
	mcpRaw, ok := cfg["mcp"]
	if !ok {
		t.Fatalf("expected mcp key in config, got: %s", data)
	}
	var mcp map[string]mcpEntry
	if err := json.Unmarshal(mcpRaw, &mcp); err != nil {
		t.Fatalf("parse mcp: %v", err)
	}
	entry, ok := mcp["knowledger"]
	if !ok {
		t.Fatalf("expected knowledger entry, got: %#v", mcp)
	}
	if entry.Type != "local" {
		t.Fatalf("expected type=local, got %q", entry.Type)
	}
	if !entry.Enabled {
		t.Fatalf("expected enabled=true")
	}
	if len(entry.Command) != 2 || entry.Command[0] != exe || entry.Command[1] != "mcp" {
		t.Fatalf("expected command=[%q, \"mcp\"], got %#v", exe, entry.Command)
	}

	schema, ok := cfg["$schema"]
	if !ok {
		t.Fatalf("expected $schema key for fresh config, got: %s", data)
	}
	var schemaVal string
	if err := json.Unmarshal(schema, &schemaVal); err != nil || schemaVal != "https://opencode.ai/config.json" {
		t.Fatalf("unexpected $schema value: %s", schema)
	}

	for _, want := range []string{
		"Checking OpenCode...",
		"Registering Knowledger MCP server in OpenCode config...",
		"Knowledger is installed for OpenCode.",
		"Restart OpenCode",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout missing %q\nstdout:\n%s", want, out.String())
		}
	}
}

func TestInstallPreservesExistingConfigKeysAndOtherMCPEntries(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "opencode.json")
	original := `{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "codegraph": {
      "type": "local",
      "command": ["codegraph", "serve", "--mcp"],
      "enabled": true
    }
  },
  "plugin": ["oh-my-openagent@latest"]
}`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(
		WithExecutablePath(func() (string, error) { return exe, nil }),
		WithHomeDir(func() (string, error) { return home, nil }),
		WithConfigPath(func() (string, error) { return configPath, nil }),
	)

	if err := installer.Install(&strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v\nconfig:\n%s", err, data)
	}

	var mcp map[string]json.RawMessage
	if err := json.Unmarshal(cfg["mcp"], &mcp); err != nil {
		t.Fatalf("parse mcp: %v", err)
	}
	if _, ok := mcp["codegraph"]; !ok {
		t.Fatalf("existing codegraph entry was removed:\n%s", data)
	}
	if _, ok := mcp["knowledger"]; !ok {
		t.Fatalf("knowledger entry not added:\n%s", data)
	}
	if _, ok := cfg["plugin"]; !ok {
		t.Fatalf("existing plugin key was removed:\n%s", data)
	}
	var schemaVal string
	if err := json.Unmarshal(cfg["$schema"], &schemaVal); err != nil || schemaVal != "https://opencode.ai/config.json" {
		t.Fatalf("$schema was altered: %s", cfg["$schema"])
	}
}

func TestInstallIdempotentRerunDoesNotRewriteConfig(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "opencode.json")
	original := `{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "knowledger": {
      "type": "local",
      "command": ["` + exe + `", "mcp"],
      "enabled": true
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	origInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(
		WithExecutablePath(func() (string, error) { return exe, nil }),
		WithHomeDir(func() (string, error) { return home, nil }),
		WithConfigPath(func() (string, error) { return configPath, nil }),
	)
	var out strings.Builder

	if err := installer.Install(&out, &strings.Builder{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	if !strings.Contains(out.String(), "already installed for OpenCode") {
		t.Fatalf("expected idempotent message, got:\n%s", out.String())
	}

	newInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if !newInfo.ModTime().Equal(origInfo.ModTime()) {
		t.Fatalf("expected config file not to be rewritten on idempotent rerun")
	}
}

func TestInstallConflictingEntryWithDifferentExecutableFailsSafely(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "opencode.json")
	original := `{
  "mcp": {
    "knowledger": {
      "type": "local",
      "command": ["/other/knowledger", "mcp"],
      "enabled": true
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(
		WithExecutablePath(func() (string, error) { return exe, nil }),
		WithHomeDir(func() (string, error) { return home, nil }),
		WithConfigPath(func() (string, error) { return configPath, nil }),
	)

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "conflicting OpenCode MCP server") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "knowledger install --opencode") {
		t.Fatalf("expected remediation hint, got: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "/other/knowledger") {
		t.Fatalf("config should be unchanged on conflict, got:\n%s", data)
	}
}

func TestInstallFallsBackToLookPathWhenExecutablePathCannotResolve(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	// Place a fake knowledger binary on a fake PATH.
	binDir := filepath.Join(home, "fakebin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeExe := filepath.Join(binDir, "knowledger")
	if err := os.WriteFile(fakeExe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	installer := NewInstaller(
		WithExecutablePath(func() (string, error) { return "", errors.New("os executable failed") }),
		WithHomeDir(func() (string, error) { return home, nil }),
		WithConfigPath(func() (string, error) { return configPath, nil }),
	)

	if err := installer.Install(&strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		MCP map[string]mcpEntry `json:"mcp"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	entry, ok := cfg.MCP["knowledger"]
	if !ok {
		t.Fatalf("expected knowledger entry, got: %s", data)
	}
	if entry.Command[0] != fakeExe && entry.Command[0] != filepath.Clean(fakeExe) {
		// LookPath may return the path as-is or cleaned; both are acceptable.
		if !strings.HasSuffix(entry.Command[0], "knowledger") {
			t.Fatalf("expected command to resolve via LookPath, got: %#v", entry.Command)
		}
	}
}

func TestInstallMissingExecutableFailsBeforeConfigMutation(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	t.Setenv("PATH", "/nonexistent/path/that/does/not/exist")

	installer := NewInstaller(
		WithExecutablePath(func() (string, error) { return "", errors.New("os executable failed") }),
		WithHomeDir(func() (string, error) { return home, nil }),
		WithConfigPath(func() (string, error) { return configPath, nil }),
	)

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("expected error when executable cannot be resolved")
	}
	if !strings.Contains(err.Error(), "could not resolve Knowledger executable path") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected config file not to be created on resolution failure, got statErr: %v", statErr)
	}
}

func TestInstallMalformedConfigFailsWithParseError(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(configPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(
		WithExecutablePath(func() (string, error) { return exe, nil }),
		WithHomeDir(func() (string, error) { return home, nil }),
		WithConfigPath(func() (string, error) { return configPath, nil }),
	)

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("expected parse error for malformed config")
	}
	if !strings.Contains(err.Error(), "failed to parse OpenCode config") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}
