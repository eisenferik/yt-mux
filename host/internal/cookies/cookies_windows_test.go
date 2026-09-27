//go:build windows

package cookies

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateSecureUsesCreateNewAndWritesCookieFile(t *testing.T) {
	path, err := CreateSecure(t.TempDir(), []Cookie{{
		Domain: ".youtube.com", Name: "SID", Value: "secret", Path: "/", Secure: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), ".youtube.com\tTRUE\t/\tTRUE\t0\tSID\tsecret") {
		t.Fatalf("unexpected cookie file: %s", payload)
	}
}

func TestCleanupStaleRemovesOnlyOldCookieFiles(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	cookieName := func(digit string) string { return "cookies-" + strings.Repeat(digit, 32) + ".txt" }
	entries := []struct {
		name    string
		age     time.Duration
		isDir   bool
		removed bool
	}{
		{cookieName("a"), 25 * time.Hour, false, true},
		{cookieName("b"), time.Hour, false, false},
		{cookieName("c"), 24 * time.Hour, false, false},
		{"cookies-old.txt", 25 * time.Hour, false, false},
		{cookieName("d"), 25 * time.Hour, true, false},
	}
	for _, entry := range entries {
		path := filepath.Join(directory, entry.name)
		if entry.isDir {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		modified := now.Add(-entry.age)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}

	if err := CleanupStale(directory, now); err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		_, err := os.Stat(filepath.Join(directory, entry.name))
		if removed := os.IsNotExist(err); removed != entry.removed {
			t.Errorf("%s: removed = %v, want %v (stat error %v)", entry.name, removed, entry.removed, err)
		}
	}
}
