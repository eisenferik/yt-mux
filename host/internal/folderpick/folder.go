package folderpick

import (
	"os"
	"path/filepath"
	"strings"
)

func usableFolder(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		parent := filepath.Dir(path)
		if parent != path {
			if st, err := os.Stat(parent); err == nil && st.IsDir() {
				return parent
			}
		}
		return ""
	}
	if info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}
