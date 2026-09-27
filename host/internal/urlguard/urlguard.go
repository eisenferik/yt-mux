package urlguard

import (
	"errors"
	"net/url"
	"slices"
	"strings"
)

var errNotAllowed = errors.New("URL is not an allowed YouTube HTTPS URL")

var videoPathNames = []string{"shorts", "live", "embed", "v", "clip"}

func Normalize(raw string) (string, error) {
	if len(raw) == 0 || len(raw) > 8192 || strings.ContainsRune(raw, '\x00') {
		return "", errNotAllowed
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return "", errNotAllowed
	}
	if port := u.Port(); port != "" && port != "443" {
		return "", errNotAllowed
	}

	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if !hostAllowed(host) || !videoURL(u, host) {
		return "", errNotAllowed
	}
	return u.String(), nil
}

func videoURL(u *url.URL, host string) bool {
	segments := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if host == "youtu.be" {
		return len(segments) == 1 && segments[0] != ""
	}
	if segments[0] == "watch" {
		return len(segments) == 1 && u.Query().Get("v") != ""
	}
	return len(segments) == 2 && slices.Contains(videoPathNames, segments[0]) && segments[1] != ""
}

func hostAllowed(host string) bool {
	return host == "youtube.com" || strings.HasSuffix(host, ".youtube.com") || host == "youtu.be"
}
