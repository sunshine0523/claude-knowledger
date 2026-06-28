package claude

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

type fakeRunner struct {
	lookPath map[string]lookPathResult
	runs     map[string]runResult

	lookedUp []string
	calls    []commandCall
}

type lookPathResult struct {
	path string
	err  error
}

type runResult struct {
	result CommandResult
	err    error
}

type commandCall struct {
	name string
	args []string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		lookPath: map[string]lookPathResult{"claude": {path: "/usr/local/bin/claude"}},
		runs: map[string]runResult{
			cmdKey("claude", "mcp", "add", "--help"):                    {},
			cmdKey("claude", "mcp", "get", "--help"):                    {},
			cmdKey("claude", "plugin", "marketplace", "add", "--help"):  {},
			cmdKey("claude", "plugin", "install", "--help"):             {},
			cmdKey("claude", "plugin", "list", "--json"):                {result: CommandResult{Stdout: "[]"}},
			cmdKey("claude", "plugin", "marketplace", "list", "--json"): {result: CommandResult{Stdout: "[]"}},
			cmdKey("claude", "mcp", "get", "knowledger"):                {err: errors.New("not found")},
		},
	}
}

func (r *fakeRunner) LookPath(file string) (string, error) {
	r.lookedUp = append(r.lookedUp, file)
	result, ok := r.lookPath[file]
	if !ok {
		return "", fmt.Errorf("%s not found", file)
	}
	return result.path, result.err
}

func (r *fakeRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})
	result, ok := r.runs[cmdKey(name, args...)]
	if !ok {
		return CommandResult{}, fmt.Errorf("unexpected command: %s", cmdKey(name, args...))
	}
	return result.result, result.err
}

func cmdKey(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), "\x00")
}

func call(name string, args ...string) commandCall {
	return commandCall{name: name, args: args}
}

func TestInstallFreshInstallRegistersMCPMarketplaceAndPlugin(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp")] = runResult{}
	marketplacePath := filepath.Join(home, ".knowledger", "claude-code", "marketplace")
	runner.runs[cmdKey("claude", "plugin", "marketplace", "add", "--scope", "user", marketplacePath)] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger")] = runResult{}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return exe, nil }), WithHomeDir(func() (string, error) { return home, nil }))
	var out strings.Builder
	var errOut strings.Builder

	if err := installer.Install(&out, &errOut); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	assertCalls(t, runner.calls, []commandCall{
		call("claude", "mcp", "add", "--help"),
		call("claude", "mcp", "get", "--help"),
		call("claude", "plugin", "marketplace", "add", "--help"),
		call("claude", "plugin", "install", "--help"),
		call("claude", "plugin", "list", "--json"),
		call("claude", "plugin", "marketplace", "list", "--json"),
		call("claude", "mcp", "get", "knowledger"),
		call("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp"),
		call("claude", "plugin", "marketplace", "add", "--scope", "user", marketplacePath),
		call("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger"),
	})
	assertFileExists(t, filepath.Join(marketplacePath, ".claude-plugin", "plugin.json"))
	assertFileExists(t, filepath.Join(marketplacePath, ".claude-plugin", "marketplace.json"))
	assertFileExists(t, filepath.Join(marketplacePath, ".mcp.json"))
	assertFileExists(t, filepath.Join(marketplacePath, "README.md"))
	assertFileExists(t, filepath.Join(marketplacePath, "skills", "knowledger", "SKILL.md"))
	assertFileExists(t, filepath.Join(marketplacePath, "skills", "git-knowledge", "SKILL.md"))
	assertFileExists(t, filepath.Join(marketplacePath, "skills", "update-knowledger", "SKILL.md"))
	assertFileExists(t, filepath.Join(marketplacePath, "hooks", "hooks.json"))
	assertFileExists(t, filepath.Join(marketplacePath, "hooks", "precheck"))
	assertFileExists(t, filepath.Join(marketplacePath, "hooks", "git-sync"))

	stdout := out.String()
	for _, want := range []string{
		"Checking Claude Code...",
		"Registering Knowledger MCP server...",
		"Installing Knowledger Claude Code plugin...",
		"Knowledger is installed for Claude Code.",
		"Verify with:",
		"  claude mcp get knowledger",
		"  claude plugin list",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q\nstdout:\n%s", want, stdout)
		}
	}
}

func TestInstallTreatsRealClaudeMissingMCPStderrAsMissing(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "get", "knowledger")] = runResult{
		result: CommandResult{Stderr: `No MCP server found with name: "knowledger". Configured servers: codegraph`},
		err:    errors.New("exit status 1"),
	}
	runner.runs[cmdKey("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp")] = runResult{}
	marketplacePath := filepath.Join(home, ".knowledger", "claude-code", "marketplace")
	runner.runs[cmdKey("claude", "plugin", "marketplace", "add", "--scope", "user", marketplacePath)] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger")] = runResult{}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return exe, nil }), WithHomeDir(func() (string, error) { return home, nil }))

	if err := installer.Install(&strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	assertCommandExists(t, runner.calls, call("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp"))
}

func TestInstallTreatsRealClaudeNoMCPNamedStderrAsMissing(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "get", "knowledger")] = runResult{
		result: CommandResult{Stderr: `No MCP server named "knowledger". Configured servers: codegraph, plugin:knowledger:kl, plugin:oh-my-claudecode:t`},
		err:    errors.New("exit status 1"),
	}
	runner.runs[cmdKey("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp")] = runResult{}
	marketplacePath := filepath.Join(home, ".knowledger", "claude-code", "marketplace")
	runner.runs[cmdKey("claude", "plugin", "marketplace", "add", "--scope", "user", marketplacePath)] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger")] = runResult{}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return exe, nil }), WithHomeDir(func() (string, error) { return home, nil }))

	if err := installer.Install(&strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	assertCommandExists(t, runner.calls, call("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp"))
}

func TestInstallFreshInstallMaterializesMarketplaceDirectoriesWith0755Permissions(t *testing.T) {
	oldUmask := syscall.Umask(0o077)
	defer syscall.Umask(oldUmask)

	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	marketplacePath := filepath.Join(home, ".knowledger", "claude-code", "marketplace")
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp")] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "marketplace", "add", "--scope", "user", marketplacePath)] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger")] = runResult{}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return exe, nil }), WithHomeDir(func() (string, error) { return home, nil }))

	if err := installer.Install(&strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	for _, dir := range []string{
		marketplacePath,
		filepath.Join(marketplacePath, ".claude-plugin"),
		filepath.Join(marketplacePath, "skills"),
		filepath.Join(marketplacePath, "skills", "knowledger"),
		filepath.Join(marketplacePath, "skills", "git-knowledge"),
		filepath.Join(marketplacePath, "skills", "update-knowledger"),
		filepath.Join(marketplacePath, "hooks"),
	} {
		assertDirMode(t, dir, 0o755)
	}

	for _, file := range []string{
		filepath.Join(marketplacePath, ".claude-plugin", "plugin.json"),
		filepath.Join(marketplacePath, ".claude-plugin", "marketplace.json"),
		filepath.Join(marketplacePath, ".mcp.json"),
		filepath.Join(marketplacePath, "README.md"),
		filepath.Join(marketplacePath, "skills", "knowledger", "SKILL.md"),
		filepath.Join(marketplacePath, "skills", "git-knowledge", "SKILL.md"),
		filepath.Join(marketplacePath, "skills", "update-knowledger", "SKILL.md"),
		filepath.Join(marketplacePath, "hooks", "hooks.json"),
	} {
		assertFileMode(t, file, 0o644)
	}

	for _, file := range []string{
		filepath.Join(marketplacePath, "hooks", "precheck"),
		filepath.Join(marketplacePath, "hooks", "git-sync"),
	} {
		assertFileMode(t, file, 0o755)
	}
}

func TestInstallIdempotentRerunSkipsExistingRegistrations(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "knowledger")
	marketplacePath := filepath.Join(home, ".knowledger", "claude-code", "marketplace")
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "get", "knowledger")] = runResult{result: CommandResult{Stdout: fmt.Sprintf("command: %s\nargs: [mcp]\n", exe)}}
	runner.runs[cmdKey("claude", "plugin", "marketplace", "list", "--json")] = runResult{result: CommandResult{Stdout: fmt.Sprintf(`[{"name":"knowledger","source":%q}]`, marketplacePath)}}
	runner.runs[cmdKey("claude", "plugin", "list", "--json")] = runResult{result: CommandResult{Stdout: `[{"id":"knowledger@knowledger","version":"dev","scope":"user","enabled":true}]`}}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return exe, nil }), WithHomeDir(func() (string, error) { return home, nil }))
	var out strings.Builder

	if err := installer.Install(&out, &strings.Builder{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	assertCalls(t, runner.calls, []commandCall{
		call("claude", "mcp", "add", "--help"),
		call("claude", "mcp", "get", "--help"),
		call("claude", "plugin", "marketplace", "add", "--help"),
		call("claude", "plugin", "install", "--help"),
		call("claude", "plugin", "list", "--json"),
		call("claude", "plugin", "marketplace", "list", "--json"),
		call("claude", "mcp", "get", "knowledger"),
	})
	assertFileExists(t, filepath.Join(marketplacePath, ".claude-plugin", "plugin.json"))
}

func TestInstallMissingClaudeExecutableFailsBeforeMutation(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	runner.lookPath["claude"] = lookPathResult{err: errors.New("not found")}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return "/abs/knowledger", nil }), WithHomeDir(func() (string, error) { return home, nil }))

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("Install returned nil error")
	}
	assertErrorContains(t, err, "Claude Code must be installed or updated")
	assertErrorContains(t, err, "knowledger install --claude")
	if len(runner.calls) != 0 {
		t.Fatalf("expected no commands after missing claude executable, got %#v", runner.calls)
	}
	assertPathDoesNotExist(t, filepath.Join(home, ".knowledger"))
}

func TestInstallUnsupportedClaudeCodeSubcommandFailsDuringPreflight(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "plugin", "marketplace", "add", "--help")] = runResult{result: CommandResult{Stderr: "unknown command marketplace"}, err: errors.New("exit status 1")}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return "/abs/knowledger", nil }), WithHomeDir(func() (string, error) { return home, nil }))

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("Install returned nil error")
	}
	assertErrorContains(t, err, "Claude Code must be installed or updated")
	assertErrorContains(t, err, "knowledger install --claude")
	assertErrorContains(t, err, "claude plugin marketplace add --help")
	assertErrorContains(t, err, "unknown command marketplace")
	if got := len(runner.calls); got != 3 {
		t.Fatalf("expected preflight to stop at failing command after 3 commands, got %d calls: %#v", got, runner.calls)
	}
	assertPathDoesNotExist(t, filepath.Join(home, ".knowledger"))
}

func TestInstallExistingConflictingMCPServerFailsSafely(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "get", "knowledger")] = runResult{result: CommandResult{Stdout: "command: /other/knowledger\nargs: [mcp]\n"}}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return "/abs/knowledger", nil }), WithHomeDir(func() (string, error) { return home, nil }))

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("Install returned nil error")
	}
	assertErrorContains(t, err, "conflicting Claude MCP server")
	assertErrorContains(t, err, "claude mcp remove knowledger")
	assertErrorContains(t, err, "knowledger install --claude")
	assertNoCommand(t, runner.calls, call("claude", "mcp", "add", "--scope", "user", "knowledger", "--", "/abs/knowledger", "mcp"))
	assertPathDoesNotExist(t, filepath.Join(home, ".knowledger"))
}

func TestInstallExistingMCPServerWithExecutablePrefixFailsSafely(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "get", "knowledger")] = runResult{result: CommandResult{Stdout: "command: /opt/bin/knowledger-old\nargs: [mcp]\n"}}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return "/opt/bin/knowledger", nil }), WithHomeDir(func() (string, error) { return home, nil }))

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("Install returned nil error")
	}
	assertErrorContains(t, err, "conflicting Claude MCP server")
	assertErrorContains(t, err, "claude mcp remove knowledger")
	assertErrorContains(t, err, "knowledger install --claude")
	assertNoCommand(t, runner.calls, call("claude", "mcp", "add", "--scope", "user", "knowledger", "--", "/opt/bin/knowledger", "mcp"))
	assertNoCommand(t, runner.calls, call("claude", "plugin", "marketplace", "add", "--scope", "user", filepath.Join(home, ".knowledger", "claude-code", "marketplace")))
	assertNoCommand(t, runner.calls, call("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger"))
	assertPathDoesNotExist(t, filepath.Join(home, ".knowledger"))
}

func TestInstallExistingConflictingMarketplaceAfterFreshMCPAddKeepsPartialSuccess(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "add", "--scope", "user", "knowledger", "--", "/abs/knowledger", "mcp")] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "marketplace", "list", "--json")] = runResult{result: CommandResult{Stdout: `[{"name":"knowledger","source":"/other/marketplace"}]`}}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return "/abs/knowledger", nil }), WithHomeDir(func() (string, error) { return home, nil }))

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("Install returned nil error")
	}
	assertErrorContains(t, err, "MCP server is installed")
	assertErrorContains(t, err, "no rollback")
	assertErrorContains(t, err, "conflicting Claude plugin marketplace")
	assertErrorContains(t, err, "claude plugin marketplace list")
	assertErrorContains(t, err, "claude plugin marketplace remove knowledger")
	assertCommandExists(t, runner.calls, call("claude", "mcp", "add", "--scope", "user", "knowledger", "--", "/abs/knowledger", "mcp"))
	assertNoCommand(t, runner.calls, call("claude", "plugin", "marketplace", "add", "--scope", "user", filepath.Join(home, ".knowledger", "claude-code", "marketplace")))
	assertNoCommand(t, runner.calls, call("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger"))
}

func TestInstallPluginInstallFailureAfterMCPSuccessKeepsPartialSuccess(t *testing.T) {
	home := t.TempDir()
	marketplacePath := filepath.Join(home, ".knowledger", "claude-code", "marketplace")
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "add", "--scope", "user", "knowledger", "--", "/abs/knowledger", "mcp")] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "marketplace", "add", "--scope", "user", marketplacePath)] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger")] = runResult{result: CommandResult{Stderr: "install exploded"}, err: errors.New("exit status 1")}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return "/abs/knowledger", nil }), WithHomeDir(func() (string, error) { return home, nil }))

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("Install returned nil error")
	}
	assertErrorContains(t, err, "MCP server is installed but plugin installation failed")
	assertErrorContains(t, err, "claude plugin install --scope user knowledger@knowledger")
	assertErrorContains(t, err, "install exploded")
	assertNoCommand(t, runner.calls, call("claude", "mcp", "remove", "knowledger"))
}

func TestInstallBundleMaterializationFailureStopsBeforeMarketplaceMutation(t *testing.T) {
	tmp := t.TempDir()
	homeFile := filepath.Join(tmp, "home-file")
	if err := os.WriteFile(homeFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := newFakeRunner()
	runner.runs[cmdKey("claude", "mcp", "add", "--scope", "user", "knowledger", "--", "/abs/knowledger", "mcp")] = runResult{}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return "/abs/knowledger", nil }), WithHomeDir(func() (string, error) { return homeFile, nil }))

	err := installer.Install(&strings.Builder{}, &strings.Builder{})
	if err == nil {
		t.Fatal("Install returned nil error")
	}
	assertErrorContains(t, err, "MCP server is installed")
	assertErrorContains(t, err, "plugin installation failed")
	assertErrorContains(t, err, "no rollback")
	assertErrorContains(t, err, "materialize")
	assertNoCommand(t, runner.calls, call("claude", "mcp", "remove", "knowledger"))
	assertNoCommand(t, runner.calls, call("claude", "plugin", "marketplace", "add", "--scope", "user", filepath.Join(homeFile, ".knowledger", "claude-code", "marketplace")))
	assertNoCommand(t, runner.calls, call("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger"))
}

func TestInstallFallsBackToLookPathWhenExecutablePathCannotResolve(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "resolved", "knowledger")
	marketplacePath := filepath.Join(home, ".knowledger", "claude-code", "marketplace")
	runner := newFakeRunner()
	runner.lookPath["knowledger"] = lookPathResult{path: exe}
	runner.runs[cmdKey("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp")] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "marketplace", "add", "--scope", "user", marketplacePath)] = runResult{}
	runner.runs[cmdKey("claude", "plugin", "install", "--scope", "user", "knowledger@knowledger")] = runResult{}

	installer := NewInstaller(WithRunner(runner), WithExecutablePath(func() (string, error) { return "", errors.New("os executable failed") }), WithHomeDir(func() (string, error) { return home, nil }))

	if err := installer.Install(&strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !reflect.DeepEqual(runner.lookedUp, []string{"knowledger", "claude"}) {
		t.Fatalf("lookups = %#v", runner.lookedUp)
	}
	assertCommandExists(t, runner.calls, call("claude", "mcp", "add", "--scope", "user", "knowledger", "--", exe, "mcp"))
}

func assertCalls(t *testing.T, got, want []commandCall) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected %s to be a file", path)
	}
}

func assertDirMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", path)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %o, want %o", path, got, want)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected %s to be a file", path)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %o, want %o", path, got, want)
	}
}

func assertPathDoesNotExist(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		t.Fatalf("expected %s not to exist", path)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error missing %q\nerror: %v", want, err)
	}
}

func assertNoCommand(t *testing.T, calls []commandCall, unwanted commandCall) {
	t.Helper()
	for _, got := range calls {
		if reflect.DeepEqual(got, unwanted) {
			t.Fatalf("unexpected command found: %#v\nall calls: %#v", unwanted, calls)
		}
	}
}

func assertCommandExists(t *testing.T, calls []commandCall, wanted commandCall) {
	t.Helper()
	for _, got := range calls {
		if reflect.DeepEqual(got, wanted) {
			return
		}
	}
	t.Fatalf("command not found: %#v\nall calls: %#v", wanted, calls)
}
