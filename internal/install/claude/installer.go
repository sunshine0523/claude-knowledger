package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	pluginbundle "github.com/kindbrave/knowledger/plugins/claude-code-knowledger"
)

const (
	mcpServerName = "knowledger"
	pluginID      = "claude-code-knowledger@knowledger"
)

var bundleFiles = []string{
	".claude-plugin/plugin.json",
	".claude-plugin/marketplace.json",
	".mcp.json",
	"README.md",
	"skills/knowledger/SKILL.md",
	"hooks/hooks.json",
	"hooks/precheck.json",
}

type CommandRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) (CommandResult, error)
}

type CommandResult struct {
	Stdout string
	Stderr string
}

type Installer struct {
	runner         CommandRunner
	executablePath func() (string, error)
	homeDir        func() (string, error)
}

type Option func(*Installer)

func WithRunner(runner CommandRunner) Option {
	return func(i *Installer) {
		if runner != nil {
			i.runner = runner
		}
	}
}

func WithExecutablePath(executablePath func() (string, error)) Option {
	return func(i *Installer) {
		if executablePath != nil {
			i.executablePath = executablePath
		}
	}
}

func WithHomeDir(homeDir func() (string, error)) Option {
	return func(i *Installer) {
		if homeDir != nil {
			i.homeDir = homeDir
		}
	}
}

func NewInstaller(opts ...Option) *Installer {
	installer := &Installer{
		runner:         execRunner{},
		executablePath: os.Executable,
		homeDir:        os.UserHomeDir,
	}
	for _, opt := range opts {
		opt(installer)
	}
	return installer
}

func (i *Installer) Install(out, errOut io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}

	runner := i.runner
	if runner == nil {
		runner = execRunner{}
	}

	executable, err := i.resolveExecutablePath(runner)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Checking Claude Code...")
	preflight, err := i.preflight(runner)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Registering Knowledger MCP server...")
	mcpInstalled, err := i.ensureMCP(runner, executable)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Installing Knowledger Claude Code plugin...")
	marketplacePath, err := i.materializeBundle()
	if err != nil {
		return pluginStageError(fmt.Errorf("failed to materialize Knowledger Claude Code plugin bundle: %w", err), mcpInstalled)
	}
	if err := ensureMarketplace(runner, preflight.marketplaces, marketplacePath); err != nil {
		return pluginStageError(err, mcpInstalled)
	}
	if err := ensurePlugin(runner, preflight.plugins); err != nil {
		return pluginStageError(err, mcpInstalled)
	}

	fmt.Fprintln(out, "Knowledger is installed for Claude Code.")
	fmt.Fprintln(out, "Verify with:")
	fmt.Fprintln(out, "  claude mcp get knowledger")
	fmt.Fprintln(out, "  claude plugin list")
	_ = errOut
	return nil
}

func (i *Installer) resolveExecutablePath(runner CommandRunner) (string, error) {
	path, err := i.executablePath()
	if err != nil || path == "" {
		path, err = runner.LookPath("knowledger")
		if err != nil {
			return "", fmt.Errorf("could not resolve Knowledger executable path: %w", err)
		}
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

type preflightResult struct {
	plugins      []pluginEntry
	marketplaces []marketplaceEntry
}

func (i *Installer) preflight(runner CommandRunner) (preflightResult, error) {
	if _, err := runner.LookPath("claude"); err != nil {
		return preflightResult{}, claudeUpdateError("claude", CommandResult{}, err)
	}

	checks := [][]string{
		{"mcp", "add", "--help"},
		{"mcp", "get", "--help"},
		{"plugin", "marketplace", "add", "--help"},
		{"plugin", "install", "--help"},
	}
	for _, args := range checks {
		result, err := runner.Run(context.Background(), "claude", args...)
		if err != nil {
			return preflightResult{}, claudeUpdateError(commandString("claude", args...), result, err)
		}
	}

	pluginList, err := runClaude(runner, "plugin", "list", "--json")
	if err != nil {
		return preflightResult{}, claudeUpdateError(commandString("claude", "plugin", "list", "--json"), pluginList, err)
	}
	marketplaceList, err := runClaude(runner, "plugin", "marketplace", "list", "--json")
	if err != nil {
		return preflightResult{}, claudeUpdateError(commandString("claude", "plugin", "marketplace", "list", "--json"), marketplaceList, err)
	}

	plugins, err := parsePluginEntries(pluginList.Stdout)
	if err != nil {
		return preflightResult{}, fmt.Errorf("failed to parse `claude plugin list --json`: %w", err)
	}
	marketplaces, err := parseMarketplaceEntries(marketplaceList.Stdout)
	if err != nil {
		return preflightResult{}, fmt.Errorf("failed to parse `claude plugin marketplace list --json`: %w", err)
	}
	return preflightResult{plugins: plugins, marketplaces: marketplaces}, nil
}

func (i *Installer) ensureMCP(runner CommandRunner, executable string) (bool, error) {
	result, err := runClaude(runner, "mcp", "get", mcpServerName)
	if err != nil {
		if isMissingMCP(result, err) {
			addResult, addErr := runClaude(runner, "mcp", "add", "--scope", "user", mcpServerName, "--", executable, "mcp")
			if addErr != nil {
				return false, commandFailedError("claude mcp add --scope user knowledger -- "+executable+" mcp", addResult, addErr)
			}
			return true, nil
		}
		return false, commandFailedError("claude mcp get knowledger", result, err)
	}

	if containsExactField(result.Stdout, executable) && containsMCPArg(result.Stdout) {
		return true, nil
	}
	return false, fmt.Errorf("found conflicting Claude MCP server named knowledger. Remove it manually, then rerun the installer:\n  claude mcp remove knowledger\n  knowledger install --claude")
}

func (i *Installer) materializeBundle() (string, error) {
	home, err := i.homeDir()
	if err != nil {
		return "", err
	}
	baseDir := filepath.Join(home, ".knowledger", "claude-code")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(baseDir, 0o755); err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp(baseDir, "marketplace-*")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(tmpDir, 0o755); err != nil {
		return "", err
	}
	moved := false
	defer func() {
		if !moved {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	for _, name := range bundleFiles {
		data, err := fs.ReadFile(pluginbundle.Bundle, name)
		if err != nil {
			return "", err
		}
		target := filepath.Join(tmpDir, filepath.FromSlash(name))
		targetDir := filepath.Dir(target)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return "", err
		}
		relDir, err := filepath.Rel(tmpDir, targetDir)
		if err != nil {
			return "", err
		}
		dir := tmpDir
		if relDir != "." {
			for _, part := range strings.Split(relDir, string(os.PathSeparator)) {
				dir = filepath.Join(dir, part)
				if err := os.Chmod(dir, 0o755); err != nil {
					return "", err
				}
			}
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return "", err
		}
		if err := os.Chmod(target, 0o644); err != nil {
			return "", err
		}
	}
	if err := os.Chmod(tmpDir, 0o755); err != nil {
		return "", err
	}

	targetDir := filepath.Join(baseDir, "marketplace")
	if err := os.RemoveAll(targetDir); err != nil {
		return "", err
	}
	if err := os.Rename(tmpDir, targetDir); err != nil {
		return "", err
	}
	moved = true
	if abs, err := filepath.Abs(targetDir); err == nil {
		targetDir = abs
	}
	return filepath.Clean(targetDir), nil
}

func ensureMarketplace(runner CommandRunner, entries []marketplaceEntry, marketplacePath string) error {
	for _, entry := range entries {
		if entry.Name != "knowledger" {
			continue
		}
		if entry.pointsTo(marketplacePath) {
			return nil
		}
		return fmt.Errorf("found conflicting Claude plugin marketplace named knowledger. Inspect or remove it manually:\n  claude plugin marketplace list\n  claude plugin marketplace remove knowledger")
	}

	result, err := runClaude(runner, "plugin", "marketplace", "add", "--scope", "user", marketplacePath)
	if err != nil {
		return commandFailedError("claude plugin marketplace add --scope user "+marketplacePath, result, err)
	}
	return nil
}

func ensurePlugin(runner CommandRunner, entries []pluginEntry) error {
	for _, entry := range entries {
		if entry.ID == pluginID {
			return nil
		}
	}

	result, err := runClaude(runner, "plugin", "install", "--scope", "user", pluginID)
	if err != nil {
		return commandFailedError("claude plugin install --scope user "+pluginID, result, err)
	}
	return nil
}

func pluginStageError(err error, mcpInstalled bool) error {
	if err == nil || !mcpInstalled {
		return err
	}
	return fmt.Errorf("MCP server is installed but plugin installation failed; no rollback was attempted: %w", err)
}

func runClaude(runner CommandRunner, args ...string) (CommandResult, error) {
	return runner.Run(context.Background(), "claude", args...)
}

func isMissingMCP(result CommandResult, err error) bool {
	text := strings.ToLower(strings.Join([]string{result.Stdout, result.Stderr, err.Error()}, "\n"))
	return strings.Contains(text, "not found") ||
		strings.Contains(text, "no such") ||
		strings.Contains(text, "does not exist") ||
		strings.Contains(text, "missing") ||
		strings.Contains(text, "no mcp server found")
}

func containsExactField(output, want string) bool {
	fields := strings.FieldsFunc(output, func(r rune) bool {
		switch r {
		case '[', ']', ',', ' ', '\t', '\n', '\r', '"', '\'':
			return true
		default:
			return false
		}
	})
	for _, field := range fields {
		if field == want {
			return true
		}
	}
	return false
}

func containsMCPArg(output string) bool {
	fields := strings.FieldsFunc(output, func(r rune) bool {
		return r == '[' || r == ']' || r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '"' || r == '\''
	})
	for _, field := range fields {
		if field == "mcp" {
			return true
		}
	}
	return false
}

func claudeUpdateError(command string, result CommandResult, err error) error {
	return fmt.Errorf("Claude Code must be installed or updated, then rerun `knowledger install --claude`: %w", commandFailedError(command, result, err))
}

func commandFailedError(command string, result CommandResult, err error) error {
	message := fmt.Sprintf("%s failed: %v", command, err)
	if strings.TrimSpace(result.Stderr) != "" {
		message += "\nstderr: " + strings.TrimSpace(result.Stderr)
	}
	return errors.New(message)
}

func commandString(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

type pluginEntry struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	InstallPath string `json:"installPath"`
}

type marketplaceEntry struct {
	Name            string `json:"name"`
	Source          string `json:"source"`
	URL             string `json:"url"`
	Repo            string `json:"repo"`
	InstallLocation string `json:"installLocation"`
}

func (e marketplaceEntry) pointsTo(path string) bool {
	for _, value := range []string{e.Source, e.URL, e.Repo, e.InstallLocation} {
		if pathMatches(value, path) {
			return true
		}
	}
	return false
}

func pathMatches(value, want string) bool {
	if value == "" {
		return false
	}
	if value == want {
		return true
	}
	cleanWant := filepath.Clean(want)
	cleanValue := filepath.Clean(value)
	if cleanValue == cleanWant {
		return true
	}
	if abs, err := filepath.Abs(value); err == nil && filepath.Clean(abs) == cleanWant {
		return true
	}
	return false
}

func parsePluginEntries(data string) ([]pluginEntry, error) {
	if strings.TrimSpace(data) == "" {
		return nil, nil
	}
	var entries []pluginEntry
	if err := json.Unmarshal([]byte(data), &entries); err == nil {
		return entries, nil
	}
	var envelope struct {
		Plugins []pluginEntry `json:"plugins"`
		Data    []pluginEntry `json:"data"`
		Items   []pluginEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return nil, err
	}
	return firstNonNil(envelope.Plugins, envelope.Data, envelope.Items), nil
}

func parseMarketplaceEntries(data string) ([]marketplaceEntry, error) {
	if strings.TrimSpace(data) == "" {
		return nil, nil
	}
	var entries []marketplaceEntry
	if err := json.Unmarshal([]byte(data), &entries); err == nil {
		return entries, nil
	}
	var envelope struct {
		Marketplaces []marketplaceEntry `json:"marketplaces"`
		Data         []marketplaceEntry `json:"data"`
		Items        []marketplaceEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return nil, err
	}
	return firstNonNil(envelope.Marketplaces, envelope.Data, envelope.Items), nil
}

func firstNonNil[T any](values ...[]T) []T {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

type execRunner struct{}

func (execRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (execRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if strings.TrimSpace(result.Stderr) != "" {
			return result, fmt.Errorf("%w: %s", err, strings.TrimSpace(result.Stderr))
		}
		return result, err
	}
	return result, nil
}
