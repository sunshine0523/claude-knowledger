package core_test

import (
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/core"
)

func TestNormalizeScopeAcceptsKnownValues(t *testing.T) {
	cases := map[string]string{
		"":        core.ScopeGlobal,
		"global":  core.ScopeGlobal,
		"GLOBAL":  core.ScopeGlobal,
		"project": core.ScopeProject,
		"Project": core.ScopeProject,
	}
	for input, want := range cases {
		got, err := core.NormalizeScope(input)
		if err != nil {
			t.Fatalf("NormalizeScope(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeScope(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeScopeRejectsUnknownValues(t *testing.T) {
	if _, err := core.NormalizeScope("team"); err == nil {
		t.Fatalf("expected error for unknown scope")
	}
}
