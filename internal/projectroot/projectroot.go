package projectroot

import (
	"errors"
	"os"
	"path/filepath"
)

const MarkerDirName = ".knowledger"

// Discover walks up from the current working directory looking for a
// `.knowledger/` directory. Returns the directory containing the marker
// (absolute path) when found. err is non-nil only on filesystem read failures.
func Discover() (string, bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	return DiscoverFrom(cwd)
}

// DiscoverFrom is Discover with the starting directory parameterised.
// It stops at the filesystem root and at the user's home directory
// (so `~/` is not treated as a project root).
func DiscoverFrom(start string) (string, bool, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", false, err
	}
	home, _ := os.UserHomeDir()
	dir := abs
	for {
		if home != "" && dir == home {
			return "", false, nil
		}
		marker := filepath.Join(dir, MarkerDirName)
		info, err := os.Stat(marker)
		if err == nil && info.IsDir() {
			return dir, true, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}
