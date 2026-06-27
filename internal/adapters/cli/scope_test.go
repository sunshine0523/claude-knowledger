package cli_test

import (
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/adapters/cli"
	"github.com/kindbrave/claude-knowledger/internal/core"
)

func TestEffectiveScopeUsesExplicitFlag(t *testing.T) {
	got, err := cli.EffectiveScope("project", false)
	if err != nil {
		t.Fatalf("EffectiveScope: %v", err)
	}
	if got != core.ScopeProject {
		t.Fatalf("got %q, want project", got)
	}
}

func TestEffectiveScopeDefaultsToProjectWhenInProject(t *testing.T) {
	got, err := cli.EffectiveScope("", true)
	if err != nil {
		t.Fatalf("EffectiveScope: %v", err)
	}
	if got != core.ScopeProject {
		t.Fatalf("got %q, want project", got)
	}
}

func TestEffectiveScopeDefaultsToGlobalWhenNotInProject(t *testing.T) {
	got, err := cli.EffectiveScope("", false)
	if err != nil {
		t.Fatalf("EffectiveScope: %v", err)
	}
	if got != core.ScopeGlobal {
		t.Fatalf("got %q, want global", got)
	}
}
