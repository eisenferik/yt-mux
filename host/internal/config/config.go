package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var languagePattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,35}$`)

type Preset struct {
	FormatSelector string
	FormatSort     string
	Container      string
}

var Presets = map[string]Preset{
	"archive": {
		FormatSelector: "bv*+ba/b",
		FormatSort:     "res,fps,hdr:12,vcodec,channels,acodec,size,br",
		Container:      "mkv",
	},
	"archive-vp9": {
		FormatSelector: "bv*+ba/b",
		FormatSort:     "res,fps,hdr:12,vcodec:vp9,channels,acodec:opus,size,br",
		Container:      "mkv",
	},
	"compat-mp4": {
		FormatSelector: "bv*[vcodec^=avc1]+ba[acodec^=mp4a]/b[vcodec^=avc1][acodec^=mp4a]",
		FormatSort:     "res,fps,size,br",
		Container:      "mp4",
	},
}

type Settings struct {
	Destination       string   `json:"destination"`
	DefaultPreset     string   `json:"defaultPreset"`
	SubtitleLanguages []string `json:"subtitleLanguages"`
	CookiesEnabled    bool     `json:"cookiesEnabled"`
	Notifications     bool     `json:"notifications"`
}

type settingsValidationError struct {
	err error
}

func (e *settingsValidationError) Error() string {
	return e.err.Error()
}

func IsSettingsValidationError(err error) bool {
	var validationError *settingsValidationError
	return errors.As(err, &validationError)
}

func LoadSettings(path string) (Settings, error) {
	var settings Settings
	if err := decodeFile(path, &settings); err != nil {
		return Settings{}, fmt.Errorf("load settings: %w", err)
	}
	if err := settings.Validate(); err != nil {
		return Settings{}, fmt.Errorf("validate settings: %w", err)
	}
	settings.Destination = filepath.Clean(settings.Destination)
	return settings, nil
}

func (s Settings) Validate() error {
	if !filepath.IsAbs(s.Destination) {
		return errors.New("destination must be an absolute path")
	}
	if strings.ContainsRune(s.Destination, '\x00') {
		return errors.New("destination must not contain NUL")
	}
	if len(s.Destination) > 1024 {
		return errors.New("destination must be at most 1024 UTF-8 bytes")
	}
	if _, ok := Presets[s.DefaultPreset]; !ok {
		return errors.New("defaultPreset is not supported")
	}
	if len(s.SubtitleLanguages) == 0 || len(s.SubtitleLanguages) > 10 {
		return errors.New("subtitleLanguages must contain between 1 and 10 entries")
	}
	seen := make(map[string]struct{}, len(s.SubtitleLanguages))
	for _, language := range s.SubtitleLanguages {
		if !languagePattern.MatchString(language) {
			return fmt.Errorf("invalid subtitle language %q", language)
		}
		key := strings.ToLower(language)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate subtitle language %q", language)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func SaveSettings(path string, settings Settings) (Settings, error) {
	if err := settings.Validate(); err != nil {
		return Settings{}, &settingsValidationError{err: err}
	}
	settings.Destination = filepath.Clean(settings.Destination)
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	payload = append(payload, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Settings{}, fmt.Errorf("create settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, "settings-*.tmp")
	if err != nil {
		return Settings{}, fmt.Errorf("create temporary settings file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return Settings{}, fmt.Errorf("secure temporary settings file: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return Settings{}, fmt.Errorf("write settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return Settings{}, fmt.Errorf("flush settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Settings{}, fmt.Errorf("close settings: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return Settings{}, fmt.Errorf("replace settings: %w", err)
	}
	return settings, nil
}

func decodeFile(path string, target any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected data after JSON document")
		}
		return err
	}
	return nil
}
