package cli_test

import (
	"strings"
	"testing"

	"github.com/kindbrave/knowledger/internal/adapters/cli"
	"github.com/kindbrave/knowledger/internal/core"
)

func TestParseKBIDsAcceptsBareIDs(t *testing.T) {
	refs, err := cli.ParseKBIDsForTest([]string{"foo", "bar"}, "", false)
	if err != nil {
		t.Fatalf("ParseKBIDs: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	for _, r := range refs {
		if r.Scope != core.ScopeGlobal {
			t.Fatalf("expected scope=global, got %q (id=%q)", r.Scope, r.ID)
		}
	}
}

func TestParseKBIDsAcceptsScopeColonID(t *testing.T) {
	refs, err := cli.ParseKBIDsForTest([]string{"project:notes", "global:default"}, "", false)
	if err != nil {
		t.Fatalf("ParseKBIDs: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].Scope != core.ScopeProject || refs[0].ID != "notes" {
		t.Fatalf("got %#v, want project:notes", refs[0])
	}
	if refs[1].Scope != core.ScopeGlobal || refs[1].ID != "default" {
		t.Fatalf("got %#v, want global:default", refs[1])
	}
}

func TestParseKBIDsRejectsBadScope(t *testing.T) {
	if _, err := cli.ParseKBIDsForTest([]string{"team:notes"}, "", false); err == nil || !strings.Contains(err.Error(), "unknown scope") {
		t.Fatalf("expected unknown-scope error, got %v", err)
	}
}

func TestParseKBIDsBareUsesProjectDefaultInProject(t *testing.T) {
	refs, err := cli.ParseKBIDsForTest([]string{"notes"}, "", true)
	if err != nil {
		t.Fatalf("ParseKBIDs: %v", err)
	}
	if len(refs) != 1 || refs[0].Scope != core.ScopeProject {
		t.Fatalf("expected project default, got %#v", refs)
	}
}
