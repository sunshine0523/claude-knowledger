package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/adapters/cli"
	"github.com/kindbrave/claude-knowledger/internal/app"
	"github.com/kindbrave/claude-knowledger/internal/config"
	"github.com/kindbrave/claude-knowledger/internal/core"
)

func TestKBCreateAndDeleteCommandsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".knowledger"), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	storePath := filepath.Join(t.TempDir(), "newkb")
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}

	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	cfg.RuntimeRegistryPath = filepath.Join(t.TempDir(), "global", "registry.json")
	svc, err := app.BuildServiceFromConfig(cfg, tmp)
	if err != nil {
		t.Fatalf("BuildServiceFromConfig: %v", err)
	}
	defer svc.Close()

	createOut := new(bytes.Buffer)
	createCmd := cli.NewRootCommand(svc)
	createCmd.SetOut(createOut)
	createCmd.SetErr(new(bytes.Buffer))
	createCmd.SetArgs([]string{
		"kb-create",
		"--id", "newkb",
		"--name", "New KB",
		"--store-type", "text",
		"--path", storePath,
		"--tag", "alpha",
	})
	if err := createCmd.Execute(); err != nil {
		t.Fatalf("kb-create returned error: %v", err)
	}
	var createPayload map[string]any
	if err := json.Unmarshal(createOut.Bytes(), &createPayload); err != nil {
		t.Fatalf("parse kb-create output %q: %v", createOut.String(), err)
	}
	if _, ok := createPayload["knowledge_base"]; !ok {
		t.Fatalf("expected knowledge_base in kb-create output, got %v", createPayload)
	}

	listOut := new(bytes.Buffer)
	listCmd := cli.NewRootCommand(svc)
	listCmd.SetOut(listOut)
	listCmd.SetErr(new(bytes.Buffer))
	listCmd.SetArgs([]string{"list-kbs"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list-kbs after create returned error: %v", err)
	}
	if !strings.Contains(listOut.String(), "newkb") {
		t.Fatalf("expected list-kbs to contain newkb, got %q", listOut.String())
	}

	deleteOut := new(bytes.Buffer)
	deleteCmd := cli.NewRootCommand(svc)
	deleteCmd.SetOut(deleteOut)
	deleteCmd.SetErr(new(bytes.Buffer))
	deleteCmd.SetArgs([]string{"kb-delete", "--id", "newkb"})
	if err := deleteCmd.Execute(); err != nil {
		t.Fatalf("kb-delete returned error: %v", err)
	}
	var delPayload struct {
		Deleted bool   `json:"deleted"`
		Scope   string `json:"scope"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(deleteOut.Bytes(), &delPayload); err != nil {
		t.Fatalf("parse kb-delete output %q: %v", deleteOut.String(), err)
	}
	if !delPayload.Deleted || delPayload.ID != "newkb" {
		t.Fatalf("unexpected kb-delete payload: %+v", delPayload)
	}
	if delPayload.Scope != core.ScopeProject {
		t.Fatalf("expected default scope=project in project dir, got %q", delPayload.Scope)
	}

	listAfter := new(bytes.Buffer)
	listCmd2 := cli.NewRootCommand(svc)
	listCmd2.SetOut(listAfter)
	listCmd2.SetErr(new(bytes.Buffer))
	listCmd2.SetArgs([]string{"list-kbs"})
	if err := listCmd2.Execute(); err != nil {
		t.Fatalf("list-kbs after delete returned error: %v", err)
	}
	if strings.Contains(listAfter.String(), "newkb") {
		t.Fatalf("expected newkb removed from list-kbs, got %q", listAfter.String())
	}
}

func TestKBCreateRejectsDuplicate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".knowledger"), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	storePath := filepath.Join(t.TempDir(), "dup")
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	cfg.RuntimeRegistryPath = filepath.Join(t.TempDir(), "global", "registry.json")
	svc, err := app.BuildServiceFromConfig(cfg, tmp)
	if err != nil {
		t.Fatalf("BuildServiceFromConfig: %v", err)
	}
	defer svc.Close()

	args := []string{"kb-create", "--id", "dup", "--store-type", "text", "--path", storePath}
	cmd1 := cli.NewRootCommand(svc)
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetErr(new(bytes.Buffer))
	cmd1.SetArgs(args)
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("first kb-create returned error: %v", err)
	}

	cmd2 := cli.NewRootCommand(svc)
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetErr(new(bytes.Buffer))
	cmd2.SetArgs(args)
	if err := cmd2.Execute(); err == nil {
		t.Fatalf("expected duplicate kb-create to fail")
	}
}
