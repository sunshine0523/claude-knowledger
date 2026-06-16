package core

import (
	"fmt"
	"strings"
)

const (
	ScopeGlobal  = "global"
	ScopeProject = "project"
)

// NormalizeScope canonicalises a scope string. Empty input maps to
// ScopeGlobal so callers that don't care about scope (back-compat paths)
// land in the global namespace by default.
func NormalizeScope(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", ScopeGlobal:
		return ScopeGlobal, nil
	case ScopeProject:
		return ScopeProject, nil
	default:
		return "", fmt.Errorf("unknown scope %q (expected %q or %q)", s, ScopeGlobal, ScopeProject)
	}
}
