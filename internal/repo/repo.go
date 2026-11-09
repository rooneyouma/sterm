package repo

import (
	"os"
	"path/filepath"
)

// Find returns the root of the git repository containing dir, or "" if none.
func Find(dir string) string {
	if dir == "" {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
