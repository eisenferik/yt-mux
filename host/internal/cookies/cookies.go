package cookies

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

const (
	netscapeHeader       = "# Netscape HTTP Cookie File\n# Generated temporarily by yt-mux.\n\n"
	maxCookies           = 500
	maxDomainBytes       = 253
	maxExpirationSeconds = 253402300799
)

type Cookie struct {
	Domain         string  `json:"domain"`
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	Path           string  `json:"path"`
	Secure         bool    `json:"secure"`
	HTTPOnly       bool    `json:"httpOnly,omitempty"`
	ExpirationDate float64 `json:"expirationDate,omitempty"`
}

func Validate(items []Cookie) error {
	if len(items) > maxCookies {
		return fmt.Errorf("cookie count exceeds %d", maxCookies)
	}
	for index, cookie := range items {
		if _, ok := normalizeDomain(cookie.Domain); !ok {
			return fmt.Errorf("cookie %d has a disallowed domain", index)
		}
		if cookie.Name == "" || len(cookie.Name) > 256 || unsafeField(cookie.Name) {
			return fmt.Errorf("cookie %d has an invalid name", index)
		}
		if len(cookie.Value) > 8192 || unsafeField(cookie.Value) {
			return fmt.Errorf("cookie %d has an invalid value", index)
		}
		if cookie.Path == "" || !strings.HasPrefix(cookie.Path, "/") || len(cookie.Path) > 2048 || unsafeField(cookie.Path) {
			return fmt.Errorf("cookie %d has an invalid path", index)
		}
		if _, ok := normalizeExpiration(cookie.ExpirationDate); !ok {
			return fmt.Errorf("cookie %d has an invalid expiration", index)
		}
	}
	return nil
}

func writeNetscape(w io.Writer, items []Cookie) error {
	buffered := bufio.NewWriter(w)
	if _, err := buffered.WriteString(netscapeHeader); err != nil {
		return err
	}
	for _, cookie := range items {
		domain, ok := normalizeDomain(cookie.Domain)
		if !ok {
			return errors.New("cookie domain is not writable")
		}
		includeSubdomains := strings.HasPrefix(domain, ".")
		if cookie.HTTPOnly {
			domain = "#HttpOnly_" + domain
		}
		expiration, ok := normalizeExpiration(cookie.ExpirationDate)
		if !ok {
			return errors.New("cookie expiration is not writable")
		}
		line := fmt.Sprintf("%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			domain, netscapeBoolean(includeSubdomains), cookie.Path, netscapeBoolean(cookie.Secure), expiration, cookie.Name, cookie.Value)
		if _, err := buffered.WriteString(line); err != nil {
			return err
		}
	}
	return buffered.Flush()
}

func netscapeBoolean(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}

func normalizeDomain(domain string) (string, bool) {
	lowered := strings.ToLower(domain)
	host := strings.TrimPrefix(lowered, ".")
	if host == "" || len(host) > maxDomainBytes {
		return "", false
	}
	if strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return "", false
	}
	for _, character := range host {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			continue
		}
		if character != '.' && character != '-' {
			return "", false
		}
	}
	if host != "youtube.com" && !strings.HasSuffix(host, ".youtube.com") &&
		host != "google.com" && !strings.HasSuffix(host, ".google.com") {
		return "", false
	}
	return lowered, true
}

func normalizeExpiration(value float64) (int64, bool) {
	if math.IsNaN(value) || value < 0 || value > maxExpirationSeconds {
		return 0, false
	}
	return int64(value), true
}

func unsafeField(value string) bool {
	return strings.ContainsAny(value, "\x00\r\n\t")
}
