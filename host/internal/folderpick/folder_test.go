package folderpick

import (
	"path/filepath"
	"testing"
)

func TestUsableFolderAcceptsExistingDirectory(t *testing.T) {
	root := t.TempDir()
	if got := usableFolder(root); got != root {
		t.Fatalf("got %q, want %q", got, root)
	}
}

func TestUsableFolderFallsBackToParent(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	if got := usableFolder(missing); got != root {
		t.Fatalf("got %q, want %q", got, root)
	}
}

func TestUsableFolderRejectsRelativeAndEmpty(t *testing.T) {
	for _, path := range []string{"", "Videos"} {
		if got := usableFolder(path); got != "" {
			t.Fatalf("expected empty for %q, got %q", path, got)
		}
	}
}
