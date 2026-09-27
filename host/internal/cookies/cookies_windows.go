//go:build windows

package cookies

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var cookieFilename = regexp.MustCompile(`^cookies-[0-9a-f]{32}\.txt$`)

func CreateSecure(directory string, items []Cookie) (string, error) {
	if err := Validate(items); err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create cookie directory: %w", err)
	}

	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate cookie filename: %w", err)
	}
	path := filepath.Join(directory, "cookies-"+hex.EncodeToString(random)+".txt")
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	securityDescriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;OW)")
	if err != nil {
		return "", fmt.Errorf("create cookie file ACL: %w", err)
	}
	attributes := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: securityDescriptor,
		InheritHandle:      0,
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		&attributes,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_TEMPORARY,
		0,
	)
	if err != nil {
		return "", fmt.Errorf("create secure cookie file: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		windows.CloseHandle(handle)
		return "", errors.New("open secure cookie file")
	}
	success := false
	defer func() {
		file.Close()
		if !success {
			os.Remove(path)
		}
	}()
	if err := writeNetscape(file, items); err != nil {
		return "", fmt.Errorf("write cookie file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("flush cookie file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close cookie file: %w", err)
	}
	success = true
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
		if entry.IsDir() || !cookieFilename.MatchString(entry.Name()) {
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
