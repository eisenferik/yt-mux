package folderpick

import (
	"os"
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

func TestValidResultRejectsFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validResult(file); err == nil {
		t.Fatal("expected a file to be rejected")
	}
	path, err := validResult(root)
	if err != nil {
		t.Fatal(err)
	}
	if path != root {
		t.Fatalf("got %q, want %q", path, root)
	}
}
