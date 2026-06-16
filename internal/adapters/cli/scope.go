package cli

import "github.com/kindbrave/knowledger/internal/core"

// EffectiveScope resolves the scope a CLI subcommand should act on.
// flag is the value of the persistent --scope flag (may be empty).
// inProject is true when the running service was started in a project directory.
func EffectiveScope(flag string, inProject bool) (string, error) {
	if flag != "" {
		return core.NormalizeScope(flag)
	}
	if inProject {
		return core.ScopeProject, nil
	}
	return core.ScopeGlobal, nil
}

// scopeFlag holds the resolved value of --scope; subcommands read from it.
// Wired by NewRootCommandWithAddressAndRunners.
var scopeFlag string

// ScopeFlagValue returns the current --scope flag value (may be "").
func ScopeFlagValue() string { return scopeFlag }
