package cookies

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteFileWritesNetscapeCookies(t *testing.T) {
	path, err := WriteFile(t.TempDir(), []Cookie{
		{Domain: ".youtube.com", Name: "SID", Value: "dummy", Path: "/", Secure: true, HTTPOnly: true, ExpirationDate: 1234567890},
		{Domain: "www.youtube.com", Name: "SID", Value: "dummy", Path: "/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := netscapeHeader +
		"#HttpOnly_.youtube.com\tTRUE\t/\tTRUE\t1234567890\tSID\tdummy\n" +
		"www.youtube.com\tFALSE\t/\tFALSE\t0\tSID\tdummy\n"
	if string(payload) != want {
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
		removed bool
	}{
		{cookieName("a"), 25 * time.Hour, true},
		{cookieName("b"), time.Hour, false},
		{cookieName("c"), 24 * time.Hour, false},
	}
	for _, entry := range entries {
		path := filepath.Join(directory, entry.name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
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
