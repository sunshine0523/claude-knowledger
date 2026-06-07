package cli_test

import (
	"bytes"
	"testing"

	"github.com/kindbrave/knowledger/internal/adapters/cli"
)

func TestRootCommandShowsSearchSubcommand(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := cli.NewRootCommand(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("search")) {
		t.Fatalf("expected help output to mention search subcommand")
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
