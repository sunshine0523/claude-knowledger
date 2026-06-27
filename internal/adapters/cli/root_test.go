package cli_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/kindbrave/knowledger/internal/adapters/cli"
)

func TestRootCommandShowsSearchAndGetSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := cli.NewRootCommand(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	for _, expected := range []string{"search", "get", "index", "mcp", "install"} {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Fatalf("expected help output to mention %s subcommand, got %s", expected, buf.String())
		}
	}
}

func TestSearchCommandShowsSearchModeFlag(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := cli.NewRootCommand(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"search", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("--search-mode")) {
		t.Fatalf("expected search help output to mention --search-mode, got %s", buf.String())
	}
}

func TestRootInstallClaudeCallsInjectedRunnerOnce(t *testing.T) {
	called := 0
	cmd := cli.NewRootCommandWithAddressAndRunners(nil, "127.0.0.1:0", func() error { return nil }, func(out, errOut io.Writer) error {
		called++
		return nil
	}, func(out, errOut io.Writer) error {
		t.Fatalf("opencode runner should not be called")
		return nil
	})
	cmd.SetArgs([]string{"install", "--claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected install runner to be called once, got %d", called)
	}
}

func TestRootInstallOpenCodeCallsInjectedRunnerOnce(t *testing.T) {
	called := 0
	cmd := cli.NewRootCommandWithAddressAndRunners(nil, "127.0.0.1:0", func() error { return nil }, func(out, errOut io.Writer) error {
		t.Fatalf("claude runner should not be called")
		return nil
	}, func(out, errOut io.Writer) error {
		called++
		return nil
	})
	cmd.SetArgs([]string{"install", "--opencode"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected install runner to be called once, got %d", called)
	}
}
