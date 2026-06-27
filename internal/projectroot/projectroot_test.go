package projectroot_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/projectroot"
)

func TestDiscoverFromFindsClosestKnowledgerDir(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "a", ".knowledger"), 0o755); err != nil {
		t.Fatalf("mkdir .knowledger: %v", err)
	}

	got, found, err := projectroot.DiscoverFrom(nested)
	if err != nil {
		t.Fatalf("DiscoverFrom returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true")
	}
	want := filepath.Join(root, "a")
	if got != want {
		t.Fatalf("got root %q, want %q", got, want)
	}
}

func TestDiscoverFromReturnsNotFoundWhenAbsent(t *testing.T) {
	root := t.TempDir()
	got, found, err := projectroot.DiscoverFrom(root)
	if err != nil {
		t.Fatalf("DiscoverFrom returned error: %v", err)
	}
	if found {
		t.Fatalf("expected found=false, got root=%q", got)
	}
}

func TestDiscoverFromStopsAtHomeBoundary(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir")
	}
	got, found, err := projectroot.DiscoverFrom(home)
	if err != nil {
		t.Fatalf("DiscoverFrom returned error: %v", err)
	}
	if found {
		t.Fatalf("expected not found at home boundary, got %q", got)
	}
}
