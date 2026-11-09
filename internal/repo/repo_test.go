package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFind(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := Find(deep); got != root {
		t.Errorf("Find(deep) = %q, want %q", got, root)
	}
	if got := Find(t.TempDir()); got != "" {
		t.Errorf("Find(outside any repo) = %q, want empty", got)
	}
	if got := Find(""); got != "" {
		t.Errorf("Find(empty) = %q, want empty", got)
	}
}
