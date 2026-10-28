package config

import (
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
	settings, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("the host cannot read the settings setup writes: %v", err)
	}
	if err := settings.Validate(); err != nil {
		t.Fatalf("the host rejects the settings setup writes: %v", err)
	}
	if _, ok := Presets[settings.DefaultPreset]; !ok {
		t.Fatalf("setup names an unknown preset: %q", settings.DefaultPreset)
	}
}

func TestSettingsValidationIdentifiesTheInvalidField(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Settings)
		want   string
	}{
		{name: "relative destination", mutate: func(s *Settings) { s.Destination = "Videos" }, want: "absolute path"},
		{name: "overlong destination", mutate: func(s *Settings) { s.Destination = `C:\` + strings.Repeat("v", 1024) }, want: "at most 1024"},
		{name: "eleven languages", mutate: func(s *Settings) { s.SubtitleLanguages = languageList(11) }, want: "at most 10"},
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

func TestSettingsValidationAcceptsZeroToTenLanguages(t *testing.T) {
	for _, count := range []int{0, 10} {
		settings := testSettings(t)
		settings.SubtitleLanguages = languageList(count)
		if err := settings.Validate(); err != nil {
			t.Fatalf("%d languages: %v", count, err)
		}
	}
}

func languageList(count int) []string {
	languages := make([]string, count)
	for index := range languages {
		languages[index] = fmt.Sprintf("ja-%d", index)
	}
	return languages
}
