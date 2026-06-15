package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestInstallClaudeCallsRunnerOnce(t *testing.T) {
	called := 0
	cmd := newInstallCommand(func(out, errOut io.Writer) error {
		called++
		if out == nil {
			t.Fatalf("expected stdout writer")
		}
		if errOut == nil {
			t.Fatalf("expected stderr writer")
		}
		return nil
	})
	cmd.SetArgs([]string{"--claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected runner to be called once, got %d", called)
	}
}

func TestInstallWithoutClaudeFails(t *testing.T) {
	errBuf := new(bytes.Buffer)
	cmd := newInstallCommand(func(out, errOut io.Writer) error {
		t.Fatalf("runner should not be called")
		return nil
	})
	cmd.SetErr(errBuf)
	cmd.SetArgs(nil)

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected install without --claude to fail")
	}
	if !strings.Contains(err.Error(), "install currently supports only --claude") {
		t.Fatalf("expected unsupported target error, got %v", err)
	}
}

func TestInstallClaudeRejectsExtraArgs(t *testing.T) {
	cmd := NewRootCommandWithAddressAndRunners(nil, "127.0.0.1:0", func() error { return nil }, func(out, errOut io.Writer) error {
		t.Fatalf("runner should not be called")
		return nil
	})
	cmd.SetArgs([]string{"install", "--claude", "extra"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected extra args to fail")
	}
	if !strings.Contains(err.Error(), "accepts 0 arg(s), received 1") {
		t.Fatalf("expected extra arg error, got %v", err)
	}
}

func TestInstallRejectsUnsupportedPublicFlags(t *testing.T) {
	cmd := newInstallCommand(func(out, errOut io.Writer) error {
		t.Fatalf("runner should not be called")
		return nil
	})
	cmd.SetArgs([]string{"--scope", "user"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected unsupported flag to fail")
	}
	if !strings.Contains(err.Error(), "unknown flag: --scope") {
		t.Fatalf("expected unknown flag error, got %v", err)
	}
}
