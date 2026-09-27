package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func testSettings(t *testing.T) Settings {
	t.Helper()
	return Settings{
		Destination:       filepath.Join(t.TempDir(), "Videos"),
		DefaultPreset:     "archive",
		SubtitleLanguages: []string{"ja", "en"},
		Notifications:     true,
	}
}

func TestPresetsMatchTheOptionsPage(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "..", "..", "extension", "options.html"))
	if err != nil {
		t.Fatal(err)
	}
	offered := map[string]bool{}
	for _, match := range regexp.MustCompile(`<option value="([^"]*)"`).FindAllStringSubmatch(string(page), -1) {
		if offered[match[1]] {
			t.Fatalf("the options page offers %q twice", match[1])
		}
		offered[match[1]] = true
	}
	for name := range Presets {
		if !offered[name] {
			t.Errorf("the options page does not offer %q", name)
		}
	}
	for name := range offered {
		if _, ok := Presets[name]; !ok {
			t.Errorf("the options page offers unknown preset %q", name)
		}
	}
}

func TestSetupDefaultsLoad(t *testing.T) {
	path := os.Getenv("YT_MUX_SETUP_SETTINGS_PATH")
	if path == "" {
		t.Skip("scripts/test-install.ps1 runs this with the settings setup writes")
	}
	if _, err := LoadSettings(path); err != nil {
		t.Fatalf("the host rejects the settings setup writes: %v", err)
	}
}

func TestLoadSettingsRejectsNonCanonicalJSON(t *testing.T) {
	payload, err := json.Marshal(testSettings(t))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "unknown field",
			payload: append([]byte(`{"unknown":true,`), payload[1:]...),
		},
		{
			name:    "trailing data",
			payload: append(append([]byte(nil), payload...), []byte("\n{}")...),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, testCase.payload, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSettings(path); err == nil {
				t.Fatal("expected invalid settings JSON to be rejected")
			}
		})
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	settings := testSettings(t)
	path := filepath.Join(t.TempDir(), "config", "settings.json")
	if _, err := SaveSettings(path, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultPreset != settings.DefaultPreset || loaded.Destination != settings.Destination {
		t.Fatalf("settings mismatch: %#v", loaded)
	}
}

func TestSaveSettingsIdentifiesValidationErrors(t *testing.T) {
	settings := Settings{DefaultPreset: "archive", SubtitleLanguages: []string{"ja"}}

	_, err := SaveSettings(filepath.Join(t.TempDir(), "settings.json"), settings)
	if !IsSettingsValidationError(err) {
		t.Fatalf("expected a settings validation error, got %v", err)
	}
}

func TestSettingsValidationIdentifiesTheInvalidField(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Settings)
		want   string
	}{
		{name: "relative destination", mutate: func(s *Settings) { s.Destination = "Videos" }, want: "absolute path"},
		{name: "destination with NUL", mutate: func(s *Settings) { s.Destination += "\x00" }, want: "NUL"},
		{name: "overlong destination", mutate: func(s *Settings) { s.Destination = `C:\` + strings.Repeat("v", 1024) }, want: "at most 1024"},
		{name: "unsupported preset", mutate: func(s *Settings) { s.DefaultPreset = "arbitrary" }, want: "defaultPreset"},
		{name: "no languages", mutate: func(s *Settings) { s.SubtitleLanguages = nil }, want: "between 1 and 10"},
		{name: "eleven languages", mutate: func(s *Settings) { s.SubtitleLanguages = languageList(11) }, want: "between 1 and 10"},
		{name: "language with a separator", mutate: func(s *Settings) { s.SubtitleLanguages = []string{"ja,en"} }, want: "invalid subtitle language"},
		{name: "overlong language", mutate: func(s *Settings) { s.SubtitleLanguages = []string{strings.Repeat("j", 36)} }, want: "invalid subtitle language"},
		{name: "duplicate language", mutate: func(s *Settings) { s.SubtitleLanguages = []string{"ja", "JA"} }, want: "duplicate subtitle language"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			settings := testSettings(t)
			testCase.mutate(&settings)

			err := settings.Validate()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("expected %q diagnostic, got %v", testCase.want, err)
			}
		})
	}
}

func TestSettingsValidationAcceptsTenLanguages(t *testing.T) {
	settings := testSettings(t)
	settings.SubtitleLanguages = languageList(10)
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
}

func languageList(count int) []string {
	languages := make([]string, count)
	for index := range languages {
		languages[index] = fmt.Sprintf("ja-%d", index)
	}
	return languages
}
