package cookies

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const netscapeHeader = "# Netscape HTTP Cookie File\n# Generated temporarily by yt-mux.\n\n"

var cookieFilename = regexp.MustCompile(`^cookies-[0-9a-f]{32}\.txt$`)

type Cookie struct {
	Domain         string  `json:"domain"`
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	Path           string  `json:"path"`
	Secure         bool    `json:"secure"`
	HTTPOnly       bool    `json:"httpOnly,omitempty"`
	ExpirationDate float64 `json:"expirationDate,omitempty"`
}

func WriteFile(directory string, items []Cookie) (string, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create cookie directory: %w", err)
	}
	random := make([]byte, 16)
	rand.Read(random)
	path := filepath.Join(directory, "cookies-"+hex.EncodeToString(random)+".txt")
	if err := os.WriteFile(path, []byte(netscape(items)), 0o600); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write cookie file: %w", err)
	}
	return path, nil
}

func CleanupStale(directory string, now time.Time) error {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := now.Add(-24 * time.Hour)
	for _, entry := range entries {
		if !cookieFilename.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(directory, entry.Name()))
	}
	return nil
}

func netscape(items []Cookie) string {
	var file strings.Builder
	file.WriteString(netscapeHeader)
	for _, cookie := range items {
		domain := cookie.Domain
		includeSubdomains := strings.HasPrefix(domain, ".")
		if cookie.HTTPOnly {
			domain = "#HttpOnly_" + domain
		}
		fmt.Fprintf(&file, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			domain, netscapeBoolean(includeSubdomains), cookie.Path, netscapeBoolean(cookie.Secure), int64(cookie.ExpirationDate), cookie.Name, cookie.Value)
	}
	return file.String()
}

func netscapeBoolean(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}
