package folderpick

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errInvalidFolder = errors.New("selected path is not a usable folder")

func usableFolder(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || len(path) > 1024 || strings.ContainsRune(path, '\x00') {
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

func validResult(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || !filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') || len(path) > 1024 {
		return "", errInvalidFolder
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errInvalidFolder
	}
	return path, nil
}
