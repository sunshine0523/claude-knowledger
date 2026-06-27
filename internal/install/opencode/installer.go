package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	mcpServerName = "knowledger"
)

// mcpEntry is the OpenCode MCP server entry shape written into opencode.json.
type mcpEntry struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

// Installer registers Knowledger with OpenCode by merging an MCP server entry
// into the user's opencode.json config file.
type Installer struct {
	executablePath func() (string, error)
	homeDir        func() (string, error)
	configPath     func() (string, error)
}

// Option configures an Installer.
type Option func(*Installer)

// WithExecutablePath overrides the Knowledger executable path resolver.
func WithExecutablePath(executablePath func() (string, error)) Option {
	return func(i *Installer) {
		if executablePath != nil {
			i.executablePath = executablePath
		}
	}
}

// WithHomeDir overrides the home directory resolver.
func WithHomeDir(homeDir func() (string, error)) Option {
	return func(i *Installer) {
		if homeDir != nil {
			i.homeDir = homeDir
		}
	}
}

// WithConfigPath overrides the opencode.json path resolver.
func WithConfigPath(configPath func() (string, error)) Option {
	return func(i *Installer) {
		if configPath != nil {
			i.configPath = configPath
		}
	}
}

// NewInstaller builds an Installer with production defaults.
func NewInstaller(opts ...Option) *Installer {
	installer := &Installer{
		executablePath: os.Executable,
		homeDir:        os.UserHomeDir,
		configPath:     defaultConfigPath,
	}
	for _, opt := range opts {
		opt(installer)
	}
	return installer
}

func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json"), nil
}

// Install resolves the Knowledger executable and ensures the OpenCode config
// registers a local MCP server named "knowledger" that runs "<exe> mcp".
func (i *Installer) Install(out, errOut io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}

	executable, err := i.resolveExecutablePath()
	if err != nil {
		return err
	}

	configPath, err := i.configPath()
	if err != nil {
		return fmt.Errorf("could not resolve OpenCode config path: %w", err)
	}

	fmt.Fprintln(out, "Checking OpenCode...")
	existing, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read OpenCode config at %s: %w", configPath, err)
	}

	fmt.Fprintln(out, "Registering Knowledger MCP server in OpenCode config...")
	wrote, err := i.ensureMCPEntry(configPath, existing, executable)
	if err != nil {
		return err
	}

	if wrote {
		fmt.Fprintln(out, "Knowledger is installed for OpenCode.")
	} else {
		fmt.Fprintln(out, "Knowledger is already installed for OpenCode.")
	}
	fmt.Fprintln(out, "Restart OpenCode (or start a new session) for the MCP server to be available.")
	fmt.Fprintln(out, "Config file:")
	fmt.Fprintln(out, "  "+configPath)
	_ = errOut
	return nil
}

func (i *Installer) resolveExecutablePath() (string, error) {
	path, err := i.executablePath()
	if err != nil || path == "" {
		looked, lookErr := exec.LookPath("knowledger")
		if lookErr != nil {
			return "", fmt.Errorf("could not resolve Knowledger executable path: %w", lookErr)
		}
		path = looked
	}
	return cleanExecutablePath(path), nil
}

func cleanExecutablePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if evaluated, err := filepath.EvalSymlinks(path); err == nil {
		path = evaluated
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

// ensureMCPEntry merges the knowledger MCP entry into the config at configPath.
// It returns wrote=true when the file was created or updated. It returns an
// error when a conflicting knowledger entry exists with a different executable.
func (i *Installer) ensureMCPEntry(configPath string, existing []byte, executable string) (bool, error) {
	top, err := parseTopLevel(existing)
	if err != nil {
		return false, fmt.Errorf("failed to parse OpenCode config at %s: %w", configPath, err)
	}

	mcpRaw, hasMCP := top["mcp"]
	mcp, err := parseMCP(mcpRaw, hasMCP)
	if err != nil {
		return false, fmt.Errorf("failed to parse existing mcp config: %w", err)
	}

	current, hasEntry := mcp[mcpServerName]
	if hasEntry {
		entry, parseErr := parseMCPEntry(current)
		if parseErr != nil {
			return false, fmt.Errorf("failed to parse existing %q MCP entry: %w", mcpServerName, parseErr)
		}
		if entryMatches(entry, executable) {
			return false, nil
		}
		return false, fmt.Errorf("found conflicting OpenCode MCP server named %s. Remove it manually from %s, then rerun the installer:\n  knowledger install --opencode", mcpServerName, configPath)
	}

	entry := mcpEntry{
		Type:    "local",
		Command: []string{executable, "mcp"},
		Enabled: true,
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return false, fmt.Errorf("failed to encode Knowledger MCP entry: %w", err)
	}
	mcp[mcpServerName] = encoded

	encodedMCP, err := json.MarshalIndent(mcp, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to encode mcp config: %w", err)
	}
	top["mcp"] = json.RawMessage(encodedMCP)

	if !hasMCP && !hasKey(top, "$schema") && len(existing) == 0 {
		top["$schema"] = json.RawMessage(`"https://opencode.ai/config.json"`)
	}

	output, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to encode OpenCode config: %w", err)
	}
	output = append(output, '\n')

	if err := i.writeAtomic(configPath, output); err != nil {
		return false, fmt.Errorf("failed to write OpenCode config at %s: %w", configPath, err)
	}
	return true, nil
}

func (i *Installer) writeAtomic(configPath string, data []byte) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".opencode.json-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	moved := false
	defer func() {
		if !moved {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, configPath); err != nil {
		return err
	}
	moved = true
	return nil
}

func parseTopLevel(existing []byte) (map[string]json.RawMessage, error) {
	if len(existing) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(existing, &top); err != nil {
		return nil, err
	}
	if top == nil {
		top = map[string]json.RawMessage{}
	}
	return top, nil
}

func parseMCP(raw json.RawMessage, hasMCP bool) (map[string]json.RawMessage, error) {
	if !hasMCP || len(raw) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var mcp map[string]json.RawMessage
	if err := json.Unmarshal(raw, &mcp); err != nil {
		return nil, err
	}
	if mcp == nil {
		mcp = map[string]json.RawMessage{}
	}
	return mcp, nil
}

func parseMCPEntry(raw json.RawMessage) (mcpEntry, error) {
	var entry mcpEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return mcpEntry{}, err
	}
	return entry, nil
}

func entryMatches(entry mcpEntry, executable string) bool {
	if entry.Type != "" && entry.Type != "local" {
		return false
	}
	hasExe := false
	hasMCP := false
	for _, c := range entry.Command {
		if c == executable {
			hasExe = true
		}
		if c == "mcp" {
			hasMCP = true
		}
	}
	return hasExe && hasMCP
}

func hasKey(top map[string]json.RawMessage, key string) bool {
	_, ok := top[key]
	return ok
}
